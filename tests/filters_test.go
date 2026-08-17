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
		{"namespace none match empty", kcontext.AlertFilters{Namespace: kcontext.NoneNamespaceFilter}, false},
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

	noNS := kcontext.StoredAlert{Labels: map[string]string{"alertname": "ClusterAlert"}}
	if !(kcontext.AlertFilters{Namespace: kcontext.NoneNamespaceFilter}).Match(noNS) {
		t.Error("none namespace filter should match alert without namespace")
	}
	if (kcontext.AlertFilters{Namespace: kcontext.NoneNamespaceFilter}).Match(alert) {
		t.Error("none namespace filter should not match alert with namespace")
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

func TestAlertFilters_matchDate_customDays(t *testing.T) {
	twoDaysAgo := time.Now().Add(-2 * 24 * time.Hour)
	fiveDaysAgo := time.Now().Add(-5 * 24 * time.Hour)

	alert2 := sampleAlert("1", "warning", "firing", "poll", "ns", "A", twoDaysAgo)
	alert5 := sampleAlert("2", "warning", "firing", "poll", "ns", "B", fiveDaysAgo)

	f := kcontext.AlertFilters{Days: 3}
	if !f.Match(alert2) {
		t.Error("alert within 3 days should match Days=3")
	}
	if f.Match(alert5) {
		t.Error("alert older than 3 days should not match Days=3")
	}
}

func TestAlertFilters_matchDate_fromTo(t *testing.T) {
	mid := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	before := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	after := time.Date(2026, 6, 15, 15, 0, 0, 0, time.UTC)

	alertMid := sampleAlert("1", "info", "firing", "webhook", "ns", "A", mid)
	alertBefore := sampleAlert("2", "info", "firing", "webhook", "ns", "B", before)
	alertAfter := sampleAlert("3", "info", "firing", "webhook", "ns", "C", after)

	f := kcontext.AlertFilters{
		From: "2026-06-15T10:00",
		To:   "2026-06-15T14:00",
	}
	if !f.Match(alertMid) {
		t.Error("alert within from/to window should match")
	}
	if f.Match(alertBefore) {
		t.Error("alert before from should not match")
	}
	if f.Match(alertAfter) {
		t.Error("alert after to should not match")
	}
}

func TestAlertFilters_matchDate_fromToRFC3339(t *testing.T) {
	alert := sampleAlert("1", "info", "firing", "webhook", "ns", "A",
		time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))

	f := kcontext.AlertFilters{
		From: "2026-06-15T10:00:00Z",
		To:   "2026-06-15T14:00:00Z",
	}
	if !f.Match(alert) {
		t.Error("RFC3339 UTC from/to should match alert in window")
	}
}

func TestAlertFilters_matchDate_todayUTC(t *testing.T) {
	now := time.Now().UTC()
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := startToday.Add(-1 * time.Hour)

	alertToday := sampleAlert("1", "info", "firing", "webhook", "ns", "A", startToday.Add(2*time.Hour))
	alertYesterday := sampleAlert("2", "info", "firing", "webhook", "ns", "B", yesterday)

	f := kcontext.AlertFilters{DateRange: "today"}
	if !f.Match(alertToday) {
		t.Error("alert from today UTC should match today filter")
	}
	if f.Match(alertYesterday) {
		t.Error("alert before UTC midnight should not match today filter")
	}
}

func TestAlertFilters_customDateHelpers(t *testing.T) {
	f := kcontext.AlertFilters{Days: 5}
	if f.DateRangeSelect() != "custom" || !f.CustomDateOpen() || f.CustomDateMode() != "days" {
		t.Fatalf("days custom = %+v select=%q open=%v mode=%q", f, f.DateRangeSelect(), f.CustomDateOpen(), f.CustomDateMode())
	}

	f2 := kcontext.AlertFilters{From: "2026-06-01", To: "2026-06-15"}
	if f2.CustomDateMode() != "calendar" || f2.FromDate() != "2026-06-01T00:00" || f2.ToDate() != "2026-06-15T23:59" {
		t.Fatalf("calendar custom = mode %q from %q to %q", f2.CustomDateMode(), f2.FromDate(), f2.ToDate())
	}

	f3 := kcontext.AlertFilters{From: "2026-06-15T10:30", To: "2026-06-15T14:45"}
	if f3.FromDate() != "2026-06-15T10:30" || f3.ToDate() != "2026-06-15T14:45" {
		t.Fatalf("datetime custom = from %q to %q", f3.FromDate(), f3.ToDate())
	}
	if f3.FromDateOnly() != "2026-06-15" || f3.FromTimeOnly() != "10:30" || f3.ToTimeOnly() != "14:45" {
		t.Fatalf("datetime parts = from %q %q to %q %q", f3.FromDateOnly(), f3.FromTimeOnly(), f3.ToDateOnly(), f3.ToTimeOnly())
	}

	enc := f.Encode()
	if !strings.Contains(enc, "range=custom") || !strings.Contains(enc, "days=5") {
		t.Errorf("Encode() = %q", enc)
	}
}

func TestParseAlertFilters_customDates(t *testing.T) {
	req := httptest.NewRequest("GET", "/?days=5&from=2026-06-01T08:00&to=2026-06-02T18:00", nil)
	f := kcontext.ParseAlertFilters(req)
	if f.Days != 5 || f.From != "2026-06-01T08:00" || f.To != "2026-06-02T18:00" {
		t.Fatalf("ParseAlertFilters() = %+v", f)
	}
	if !f.Active() {
		t.Error("custom date filters should be active")
	}
	enc := f.Encode()
	if !strings.Contains(enc, "days=5") || !strings.Contains(enc, "from=") || !strings.Contains(enc, "to=") {
		t.Errorf("Encode() = %q", enc)
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

func TestRankAlertsByNamespace(t *testing.T) {
	now := time.Now()
	alerts := []kcontext.StoredAlert{
		sampleAlert("1", "critical", "firing", "webhook", "kube-system", "A", now),
		sampleAlert("2", "warning", "firing", "webhook", "openshift-monitoring", "B", now),
		sampleAlert("3", "warning", "firing", "webhook", "openshift-monitoring", "C", now),
		sampleAlert("4", "info", "firing", "webhook", "", "D", now),
	}

	got := kcontext.RankAlertsByNamespace(alerts)
	if len(got) != 3 {
		t.Fatalf("RankAlertsByNamespace() len = %d, want 3", len(got))
	}
	if got[0].Namespace != "openshift-monitoring" || got[0].Count != 2 {
		t.Fatalf("first rank = %+v, want openshift-monitoring:2", got[0])
	}
	if got[1].Namespace != "" || got[1].Count != 1 {
		t.Fatalf("second rank = %+v, want empty namespace:1", got[1])
	}
	if got[2].Namespace != "kube-system" || got[2].Count != 1 {
		t.Fatalf("third rank = %+v, want kube-system:1", got[2])
	}
}

func TestRankAlertsByNamespace_respectsFilters(t *testing.T) {
	now := time.Now()
	alerts := []kcontext.StoredAlert{
		sampleAlert("1", "critical", "firing", "webhook", "kube-system", "A", now),
		sampleAlert("2", "warning", "firing", "webhook", "openshift-monitoring", "B", now),
		sampleAlert("3", "warning", "firing", "webhook", "openshift-monitoring", "C", now),
	}

	filtered := kcontext.FilterAlerts(alerts, kcontext.AlertFilters{Severity: "critical"})
	got := kcontext.RankAlertsByNamespace(filtered)
	if len(got) != 1 || got[0].Namespace != "kube-system" || got[0].Count != 1 {
		t.Fatalf("RankAlertsByNamespace(filtered) = %+v, want kube-system:1 only", got)
	}
}

func TestFilterAlerts_sortsByUpdated(t *testing.T) {
	old := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)
	new := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)

	alerts := []kcontext.StoredAlert{
		{ID: "oldest", ReceivedAt: old, UpdatedAt: old},
		{ID: "newest", ReceivedAt: new, UpdatedAt: new},
		{ID: "middle", ReceivedAt: mid, UpdatedAt: mid},
	}

	got := kcontext.FilterAlerts(alerts, kcontext.AlertFilters{})
	if len(got) != 3 {
		t.Fatalf("FilterAlerts() len = %d, want 3", len(got))
	}
	if got[0].ID != "newest" || got[1].ID != "middle" || got[2].ID != "oldest" {
		t.Fatalf("FilterAlerts() order = [%s %s %s], want [newest middle oldest]",
			got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestPaginateAlerts(t *testing.T) {
	alerts := make([]kcontext.StoredAlert, 55)
	for i := range alerts {
		alerts[i] = kcontext.StoredAlert{ID: string(rune('a' + i))}
	}

	page, totalPages, pageNum := kcontext.PaginateAlerts(alerts, 2, 50)
	if len(page) != 5 || totalPages != 2 || pageNum != 2 {
		t.Fatalf("PaginateAlerts page 2 = len %d totalPages %d pageNum %d", len(page), totalPages, pageNum)
	}

	empty, tp, pn := kcontext.PaginateAlerts(nil, 1, 50)
	if empty != nil || tp != 1 || pn != 1 {
		t.Fatalf("empty paginate = %v %d %d", empty, tp, pn)
	}

	_, _, pnHigh := kcontext.PaginateAlerts(alerts, 99, 50)
	if pnHigh != 2 {
		t.Errorf("page beyond total should clamp to %d, got %d", 2, pnHigh)
	}
}

func TestStoredAlert_UpdatedDisplayTime(t *testing.T) {
	updated := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	received := time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC)
	a := kcontext.StoredAlert{ReceivedAt: received, UpdatedAt: updated}
	if !a.UpdatedDisplayTime().Equal(updated) {
		t.Fatalf("UpdatedDisplayTime with updatedAt = %v, want %v", a.UpdatedDisplayTime(), updated)
	}
	b := kcontext.StoredAlert{ReceivedAt: received}
	if !b.UpdatedDisplayTime().Equal(received) {
		t.Fatalf("UpdatedDisplayTime without updatedAt = %v, want %v", b.UpdatedDisplayTime(), received)
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
	got = kcontext.NamespacesForFilter(alerts, kcontext.NoneNamespaceFilter)
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("none filter should not append sentinel, got %v", got)
	}
}

func TestAlertsHaveEmptyNamespace(t *testing.T) {
	with := []kcontext.StoredAlert{{Labels: map[string]string{"namespace": "a"}}}
	if kcontext.AlertsHaveEmptyNamespace(with) {
		t.Error("expected false when all alerts have namespace")
	}
	mixed := append(with, kcontext.StoredAlert{Labels: map[string]string{"alertname": "x"}})
	if !kcontext.AlertsHaveEmptyNamespace(mixed) {
		t.Error("expected true when some alerts lack namespace")
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
	first := d.FirstPageLink()
	if first != "/?severity=critical" {
		t.Errorf("FirstPageLink() = %q, want page 1 (no page param)", first)
	}
	last := d.LastPageLink()
	if !strings.Contains(last, "page=3") || !strings.Contains(last, "severity=critical") {
		t.Errorf("LastPageLink() = %q", last)
	}
	if (kcontext.AlertsPageData{Page: 1, TotalPages: 3}).FirstPageLink() != "" {
		t.Error("page 1 should have empty first link")
	}
	if (kcontext.AlertsPageData{Page: 3, TotalPages: 3}).LastPageLink() != "" {
		t.Error("last page should have empty last link")
	}
	if (kcontext.AlertsPageData{Page: 1}).PrevPageLink() != "" {
		t.Error("page 1 should have empty prev link")
	}
	none := kcontext.AlertsPageData{}.NoneNamespaceLink()
	if !strings.Contains(none, "namespace=__none__") {
		t.Errorf("NoneNamespaceLink() = %q", none)
	}
}
