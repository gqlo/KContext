package kcontext_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gqlo/kcontext"
)

func assertHeaderIntro(t *testing.T, body string) {
	t.Helper()

	for _, want := range []string{
		"OpenShift Virtualization Performance and Scale",
		"What it is:",
		"persistent alert history",
		"What it does:",
		"stores them in Redis",
		"future debugging",
		`href="https://github.com/gqlo/KContext"`,
		"github.com/gqlo/KContext",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("header intro missing %q", want)
		}
	}
}

func TestHandleAlertsPage_includesHeaderIntro(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	s.HandleAlertsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	assertHeaderIntro(t, body)
	if !strings.Contains(body, "newest first") {
		t.Error("dashboard header should include alert list meta line")
	}
}

func TestHandleAlertDetail_includesHeaderIntro(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	if err := s.Store().Save(ctx, kcontext.Alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "warning",
			"namespace": "demo",
		},
	}); err != nil {
		t.Fatal(err)
	}

	alerts, err := s.Store().List(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	req := httptest.NewRequest(http.MethodGet, "/alert?id="+alerts[0].ID, nil)
	rec := httptest.NewRecorder()
	s.HandleAlertDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	assertHeaderIntro(t, body)
	if !strings.Contains(body, "Alert detail") {
		t.Error("detail page header should include page meta line")
	}
	if !strings.Contains(body, "TestAlert") {
		t.Error("detail page should include alert name")
	}
}
