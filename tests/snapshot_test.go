package kcontext_test

import (
	"context"
	"testing"
	"time"

	"github.com/gqlo/kcontext"
)

func TestAlertSnapshotCache_refreshAndFilter(t *testing.T) {
	store := testStore(t)
	cache := kcontext.NewAlertSnapshotCacheForTest(store)
	defer cache.Stop()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := store.Save(ctx, kcontext.Alert{
			Status: "firing",
			Labels: map[string]string{
				"alertname": "SnapTest",
				"severity":  "critical",
				"namespace": "kube-system",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := store.Save(ctx, kcontext.Alert{
			Status: "firing",
			Labels: map[string]string{
				"alertname": "SnapTest",
				"severity":  "warning",
				"namespace": "demo",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	cache.RefreshNow(ctx)
	if got := cache.Total(); got != 5 {
		t.Fatalf("Total() = %d, want 5", got)
	}

	view := cache.FilteredView(kcontext.AlertFilters{Severity: "critical"})
	if len(view.Alerts) != 3 {
		t.Fatalf("filtered critical alerts = %d, want 3", len(view.Alerts))
	}
	if len(view.NamespaceRanks) != 1 || view.NamespaceRanks[0].Namespace != "kube-system" {
		t.Fatalf("NamespaceRanks = %+v, want kube-system only", view.NamespaceRanks)
	}

	view2 := cache.FilteredView(kcontext.AlertFilters{Severity: "critical"})
	if len(view2.Alerts) != 3 {
		t.Fatal("expected filter stats cache hit on second call")
	}
}

func TestAlertSnapshotCache_markDirtyRefreshes(t *testing.T) {
	store := testStore(t)
	cache := kcontext.NewAlertSnapshotCacheForTest(store)
	cache.StartForTest()
	defer cache.Stop()

	cache.RefreshNow(context.Background())
	if cache.Total() != 0 {
		t.Fatalf("initial Total() = %d, want 0", cache.Total())
	}

	if err := store.Save(context.Background(), kcontext.Alert{
		Status:  "firing",
		Labels:  map[string]string{"alertname": "A", "severity": "info", "namespace": "demo"},
	}); err != nil {
		t.Fatal(err)
	}
	cache.MarkDirty()

	deadline := time.Now().Add(2 * time.Second)
	for cache.Total() < 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if cache.Total() != 1 {
		t.Fatalf("Total() after MarkDirty = %d, want 1", cache.Total())
	}
}

func TestAlertFilters_statsKeyIgnoresPage(t *testing.T) {
	base := kcontext.AlertFilters{Severity: "critical", Page: 1}
	withPage := kcontext.AlertFilters{Severity: "critical", Page: 4}
	if base.StatsKey() != withPage.StatsKey() {
		t.Fatalf("StatsKey() = %q vs %q, want equal", base.StatsKey(), withPage.StatsKey())
	}
}
