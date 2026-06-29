package kcontext_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gqlo/kcontext"
	"github.com/redis/go-redis/v9"
)

func TestSaveAndList(t *testing.T) {
	s := testStore(t, 500)
	ctx := context.Background()

	err := s.Save(ctx, kcontext.Alert{
		Status: "firing",
		Labels: map[string]string{"alertname": "HighCPU", "severity": "warning"},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	alerts, err := s.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("List len = %d, want 1", len(alerts))
	}
	if alerts[0].Source != "webhook" || alerts[0].Status != "firing" {
		t.Fatalf("stored alert = %+v", alerts[0])
	}
	if alerts[0].Labels == nil || alerts[0].Annotations == nil {
		t.Error("nil maps should be normalized to empty maps")
	}
}

func TestList_respectsLimit(t *testing.T) {
	s := testStore(t, 500)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := s.Save(ctx, kcontext.Alert{Status: "firing", Labels: map[string]string{"alertname": "A"}}); err != nil {
			t.Fatal(err)
		}
	}

	alerts, err := s.List(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 2 {
		t.Fatalf("List limit 2 = %d alerts", len(alerts))
	}
}

func TestSave_trimsToMaxLen(t *testing.T) {
	s := testStore(t, 3)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := s.Save(ctx, kcontext.Alert{
			Status: "firing",
			Labels: map[string]string{"alertname": string(rune('A' + i))},
		}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.Len(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("Len = %d, want 3", n)
	}
}

func TestGet(t *testing.T) {
	s := testStore(t, 500)
	ctx := context.Background()

	if err := s.Save(ctx, kcontext.Alert{Status: "firing", Labels: map[string]string{"alertname": "X"}}); err != nil {
		t.Fatal(err)
	}

	list, _ := s.List(ctx, 1)
	if len(list) != 1 {
		t.Fatal("expected one alert")
	}

	got, err := s.Get(ctx, list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != list[0].ID {
		t.Fatalf("Get() = %+v", got)
	}

	missing, err := s.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Errorf("Get nonexistent = %+v, want nil", missing)
	}
}

func TestSyncPolled_newAndDedup(t *testing.T) {
	s := testStore(t, 500)
	ctx := context.Background()

	fp := "abc123"
	polled := []kcontext.PolledAlert{{
		Fingerprint: fp,
		Alert: kcontext.Alert{
			Status:      "firing",
			Fingerprint: fp,
			Labels:      map[string]string{"alertname": "DiskFull", "severity": "critical"},
		},
	}}

	newCount, resolved, err := s.SyncPolled(ctx, polled)
	if err != nil {
		t.Fatal(err)
	}
	if newCount != 1 || resolved != 0 {
		t.Fatalf("first sync: new=%d resolved=%d", newCount, resolved)
	}

	newCount, resolved, err = s.SyncPolled(ctx, polled)
	if err != nil {
		t.Fatal(err)
	}
	if newCount != 0 || resolved != 0 {
		t.Fatalf("duplicate sync should skip: new=%d resolved=%d", newCount, resolved)
	}

	alerts, _ := s.List(ctx, 10)
	if len(alerts) != 1 {
		t.Fatalf("dedup should keep one list entry, got %d", len(alerts))
	}
}

func TestSyncPolled_resolvedWhenMissing(t *testing.T) {
	s := testStore(t, 500)
	ctx := context.Background()

	fp := "deadbeef"
	polled := []kcontext.PolledAlert{{
		Fingerprint: fp,
		Alert: kcontext.Alert{
			Status:      "firing",
			Fingerprint: fp,
			Labels:      map[string]string{"alertname": "OOMKill", "severity": "warning"},
		},
	}}

	if _, _, err := s.SyncPolled(ctx, polled); err != nil {
		t.Fatal(err)
	}

	newCount, resolved, err := s.SyncPolled(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if newCount != 0 || resolved != 1 {
		t.Fatalf("missing alert: new=%d resolved=%d", newCount, resolved)
	}

	alerts, _ := s.List(ctx, 10)
	if len(alerts) != 2 {
		t.Fatalf("want firing + resolved entries, got %d", len(alerts))
	}
	if alerts[0].Status != "resolved" {
		t.Errorf("newest entry status = %q, want resolved", alerts[0].Status)
	}
}

func TestNewStoredFromAlert(t *testing.T) {
	start := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	stored := kcontext.NewStoredFromAlert(kcontext.Alert{
		Status:      "firing",
		Fingerprint: "fp1",
		Labels:      map[string]string{"alertname": "Test"},
		StartsAt:    start,
	}, "poll")

	if stored.Source != "poll" || stored.Fingerprint != "fp1" || stored.ID == "" {
		t.Fatalf("NewStoredFromAlert = %+v", stored)
	}
	if stored.ReceivedAt.IsZero() {
		t.Error("ReceivedAt should be set")
	}
}

func TestList_legacyReceivedAtFormat(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := context.Background()
	legacy := `{"id":"legacy-1","received_at":"2026-06-29T10:00:00","source":"webhook","status":"firing","labels":{},"annotations":{}}`
	if err := rdb.LPush(ctx, "kcontext:alerts", legacy).Err(); err != nil {
		t.Fatal(err)
	}

	s := kcontext.NewAlertStoreWithRedis(rdb, 500)
	alerts, err := s.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("List len = %d, want 1", len(alerts))
	}
	want := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	if !alerts[0].ReceivedAt.Equal(want) {
		t.Fatalf("ReceivedAt = %v, want %v", alerts[0].ReceivedAt, want)
	}
}
