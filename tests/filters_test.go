package kcontext_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gqlo/kcontext"
)

func TestParseAlertFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/?severity=critical&status=firing&page=2&range=7d", nil)
	f := kcontext.ParseAlertFilters(req)

	if f.Severity != "critical" || f.Status != "firing" || f.Page != 2 || f.DateRange != "7d" {
		t.Fatalf("ParseAlertFilters() = %+v", f)
	}

	reqZero := httptest.NewRequest("GET", "/?page=0", nil)
	if f2 := kcontext.ParseAlertFilters(reqZero); f2.Page != 1 {
		t.Errorf("page 0 should default to 1, got %d", f2.Page)
	}
}

func TestAlertFilters_Encode(t *testing.T) {
	got := kcontext.AlertFilters{
		Severity:  "warning",
		Namespace: "openshift-monitoring",
		Page:      3,
	}.Encode()

	if got != "namespace=openshift-monitoring&page=3&severity=warning" &&
		got != "page=3&severity=warning&namespace=openshift-monitoring" {
		t.Errorf("Encode() = %q", got)
	}
}

func TestAlertFilters_Active(t *testing.T) {
	if (kcontext.AlertFilters{}).Active() {
		t.Error("empty filters should not be active")
	}
	if !(kcontext.AlertFilters{Severity: "critical"}).Active() {
		t.Error("severity filter should be active")
	}
}

func TestAlertFilters_Match(t *testing.T) {
	now := time.Now()
	alert := sampleAlert("1", "critical", "firing", "webhook", "kube-system", "HighCPU", now)

	tests := []struct {
		name    string
		filters kcontext.AlertFilters
		want    bool
	}{
		{"no filters", kcontext.AlertFilters{}, true},
		{"severity match", kcontext.AlertFilters{Severity: "critical"}, true},
		{"severity mismatch", kcontext.AlertFilters{Severity: "warning"}, false},
		{"status match", kcontext.AlertFilters{Status: "firing"}, true},
		{"source match", kcontext.AlertFilters{Source: "webhook"}, true},
		{"namespace match", kcontext.AlertFilters{Namespace: "kube-system"}, true},
		{"alertname substring", kcontext.AlertFilters{Alertname: "highcpu"}, true},
		{"alertname miss", kcontext.AlertFilters{Alertname: "disk"}, false},
		{"7d range", kcontext.AlertFilters{DateRange: "7d"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filters.Match(alert); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlertFilters_matchDate_oldAlert(t *testing.T) {
	old := time.Now().Add(-8 * 24 * time.Hour)
	alert := sampleAlert("1", "warning", "firing", "poll", "ns", "A", old)

	if (kcontext.AlertFilters{DateRange: "7d"}).Match(alert) {
		t.Error("alert older than 7d should not match 7d range")
	}
	if !(kcontext.AlertFilters{DateRange: "30d"}).Match(alert) {
		t.Error("alert within 30d should match 30d range")
	}
}

func TestFilterAlerts(t *testing.T) {
	now := time.Now()
	alerts := []kcontext.StoredAlert{
		sampleAlert("1", "critical", "firing", "webhook", "a", "X", now),
		sampleAlert("2", "warning", "firing", "poll", "b", "Y", now),
	}

	got := kcontext.FilterAlerts(alerts, kcontext.AlertFilters{Severity: "critical"})
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("FilterAlerts() = %+v", got)
	}
}

func TestPaginateAlerts(t *testing.T) {
	alerts := make([]kcontext.StoredAlert, 55)
	for i := range alerts {
		alerts[i] = kcontext.StoredAlert{ID: string(rune('a' + i))}
	}

	page, totalPages, pageNum := kcontext.PaginateAlerts(alerts, 2)
	if len(page) != 5 || totalPages != 2 || pageNum != 2 {
		t.Fatalf("PaginateAlerts page 2 = len %d totalPages %d pageNum %d", len(page), totalPages, pageNum)
	}

	empty, tp, pn := kcontext.PaginateAlerts(nil, 1)
	if empty != nil || tp != 1 || pn != 1 {
		t.Fatalf("empty paginate = %v %d %d", empty, tp, pn)
	}

	_, _, pnHigh := kcontext.PaginateAlerts(alerts, 99)
	if pnHigh != 2 {
		t.Errorf("page beyond total should clamp to %d, got %d", 2, pnHigh)
	}
}

func TestStoredAlert_helpers(t *testing.T) {
	a := kcontext.StoredAlert{
		Status: "firing",
		Labels: map[string]string{
			"severity":  "Critical",
			"alertname": "Test",
			"namespace": "openshift-monitoring",
			"pod":       "prometheus-0",
		},
		Annotations: map[string]string{
			"runbook_url": "https://example.com/runbook",
		},
	}

	if a.Namespace() != "openshift-monitoring" {
		t.Errorf("Namespace() = %q", a.Namespace())
	}
	if a.Severity() != "critical" {
		t.Errorf("Severity() = %q", a.Severity())
	}
	if a.Pod() != "prometheus-0" {
		t.Errorf("Pod() = %q", a.Pod())
	}
	if a.RunbookURL() != "https://example.com/runbook" {
		t.Errorf("RunbookURL() = %q", a.RunbookURL())
	}
	if a.RowClass() != "row-firing row-severity-critical" {
		t.Errorf("RowClass() = %q", a.RowClass())
	}

	resolved := kcontext.StoredAlert{Status: "resolved"}
	if resolved.RowClass() != "row-resolved" {
		t.Errorf("resolved RowClass() = %q", resolved.RowClass())
	}
}

func TestUniqueNamespaces(t *testing.T) {
	alerts := []kcontext.StoredAlert{
		{Labels: map[string]string{"namespace": "b"}},
		{Labels: map[string]string{"namespace": "a"}},
		{Labels: map[string]string{"namespace": "b"}},
	}
	got := kcontext.UniqueNamespaces(alerts)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("UniqueNamespaces() = %v", got)
	}
}

func TestNamespacesForFilter_includesSelected(t *testing.T) {
	alerts := []kcontext.StoredAlert{{Labels: map[string]string{"namespace": "a"}}}
	got := kcontext.NamespacesForFilter(alerts, "missing-ns")
	if len(got) != 2 {
		t.Fatalf("expected selected namespace appended, got %v", got)
	}
}

func TestAlertsPageData_links(t *testing.T) {
	d := kcontext.AlertsPageData{
		Page:       2,
		TotalPages: 3,
		Filters:    kcontext.AlertFilters{Severity: "critical"},
	}
	prev := d.PrevPageLink()
	if prev != "/?severity=critical" {
		t.Errorf("PrevPageLink() = %q, want page 1 (no page param)", prev)
	}
	next := d.NextPageLink()
	if !strings.Contains(next, "page=3") || !strings.Contains(next, "severity=critical") {
		t.Errorf("NextPageLink() = %q", next)
	}
	if (kcontext.AlertsPageData{Page: 1}).PrevPageLink() != "" {
		t.Error("page 1 should have empty prev link")
	}
}
