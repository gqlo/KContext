package kcontext_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gqlo/kcontext"
)

func TestHandleWebhook_methodNotAllowed(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rec := httptest.NewRecorder()

	s.HandleWebhook(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleWebhook_invalidJSON(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	s.HandleWebhook(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleWebhook_savesAlerts(t *testing.T) {
	s := testServer(t)
	body := `{"alerts":[{"status":"firing","labels":{"alertname":"HighCPU","severity":"warning","namespace":"test"},"annotations":{"summary":"CPU high"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	rec := httptest.NewRecorder()

	s.HandleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	alerts, err := s.Store().List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 stored alert, got %d", len(alerts))
	}
	if alerts[0].Labels["alertname"] != "HighCPU" {
		t.Errorf("alertname = %q", alerts[0].Labels["alertname"])
	}
}

func TestHandleAlertsPage_GET(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	_ = s.Store().Save(ctx, kcontext.Alert{
		Status: "firing",
		Labels: map[string]string{"alertname": "TestAlert", "severity": "info", "namespace": "demo"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.HandleAlertsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "TestAlert") {
		t.Error("dashboard body should contain alert name")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestHandleAlertsPage_namespaceRanksRespectFilters(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	for _, a := range []kcontext.Alert{
		{Status: "firing", Labels: map[string]string{"severity": "critical", "alertname": "CritA", "namespace": "kube-system"}},
		{Status: "firing", Labels: map[string]string{"severity": "warning", "alertname": "WarnB", "namespace": "openshift-monitoring"}},
		{Status: "firing", Labels: map[string]string{"severity": "warning", "alertname": "WarnC", "namespace": "openshift-monitoring"}},
	} {
		if err := s.Store().Save(ctx, a); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/?severity=critical", nil)
	rec := httptest.NewRecorder()
	s.HandleAlertsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Count(body, "kube-system") < 1 {
		t.Error("sidebar should include kube-system for critical alerts")
	}
	if strings.Contains(body, "openshift-monitoring") {
		t.Error("sidebar should not include openshift-monitoring when filtered to critical only")
	}
}

func TestHandleAlertsPage_methodNotAllowed(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	s.HandleAlertsPage(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleAlertsPage_paginationLastLink(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	for i := 0; i < 201; i++ {
		if err := s.Store().Save(ctx, kcontext.Alert{
			Status: "firing",
			Labels: map[string]string{
				"alertname": "PaginateTest",
				"severity":  "info",
				"namespace": "demo",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.HandleAlertsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `href="/?page=2">Last</a>`) {
		t.Error("page 1 should link Last to the final page")
	}
	if strings.Count(body, `class="disabled">Last</span>`) != 0 {
		t.Error("page 1 should not disable Last")
	}
}

func TestSlackEnabled(t *testing.T) {
	s := kcontext.NewServer(nil, "", "")
	if s.SlackEnabled() {
		t.Error("empty server should not have slack enabled")
	}
	s = kcontext.NewServer(nil, "xoxb-test", "C123")
	if !s.SlackEnabled() {
		t.Error("token + channel should enable slack")
	}
}
