package kcontext

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultAlertsPerPage = 200
const maxAlertsPerPage = 200

// NoneNamespaceFilter is the query value for alerts with no namespace label.
const NoneNamespaceFilter = "__none__"

type AlertFilters struct {
	Severity  string
	Status    string
	Source    string
	DateRange string
	Date      string // legacy single-day filter (YYYY-MM-DD)
	Days      int    // past N days (overrides DateRange presets when > 0)
	From      string // start of time window (YYYY-MM-DD, YYYY-MM-DDTHH:MM, or RFC3339)
	To        string // end of time window
	Namespace string
	Alertname string
	Page      int
	PerPage   int
}

type AlertsPageData struct {
	Alerts                  []StoredAlert
	Count                   int
	Filtered                int
	Total                   int
	Filters                 AlertFilters
	Namespaces              []string
	NamespaceRanks          []NamespaceAlertRank
	HasEmptyNamespaceAlerts bool
	Page                    int
	TotalPages              int
	PageStart               int
	PageEnd                 int
	Cluster                 ClusterMeta
}

// NamespaceAlertRank is alert count for one namespace, sorted by count in RankAlertsByNamespace.
type NamespaceAlertRank struct {
	Namespace string
	Count     int
}

func (d AlertsPageData) PageLink(page int) string {
	f := d.Filters
	f.Page = page
	return "/?" + f.Encode()
}

func (d AlertsPageData) PrevPageLink() string {
	if d.Page <= 1 {
		return ""
	}
	return d.PageLink(d.Page - 1)
}

func (d AlertsPageData) FirstPageLink() string {
	if d.Page <= 1 {
		return ""
	}
	return d.PageLink(1)
}

func (d AlertsPageData) NextPageLink() string {
	if d.Page >= d.TotalPages {
		return ""
	}
	return d.PageLink(d.Page + 1)
}

func (d AlertsPageData) LastPageLink() string {
	if d.Page >= d.TotalPages || d.TotalPages <= 1 {
		return ""
	}
	return d.PageLink(d.TotalPages)
}

func (d AlertsPageData) NamespaceLink(ns string) string {
	f := d.Filters
	f.Namespace = ns
	f.Page = 1
	return "/?" + f.Encode()
}

func (d AlertsPageData) NoneNamespaceLink() string {
	return d.NamespaceLink(NoneNamespaceFilter)
}

func (f AlertFilters) NamespaceIsNone() bool {
	return f.Namespace == NoneNamespaceFilter
}

func (AlertFilters) NoneNamespaceFilterValue() string {
	return NoneNamespaceFilter
}

func (d AlertsPageData) AlertDetailLink(id string) string {
	return "/alert?id=" + url.QueryEscape(id) + "&" + d.Filters.Encode()
}

// StatsKey identifies filter parameters that affect counts and sidebar stats (not page).
func (f AlertFilters) StatsKey() string {
	ff := f
	ff.Page = 0
	ff.PerPage = 0
	return ff.Encode()
}

func (f AlertFilters) Encode() string {
	v := url.Values{}
	if f.Severity != "" {
		v.Set("severity", f.Severity)
	}
	if f.Status != "" {
		v.Set("status", f.Status)
	}
	if f.Source != "" {
		v.Set("source", f.Source)
	}
	if f.Days > 0 || f.From != "" || f.To != "" {
		v.Set("range", "custom")
	} else if f.DateRange != "" {
		v.Set("range", f.DateRange)
	}
	if f.Date != "" {
		v.Set("date", f.Date)
	}
	if f.Days > 0 {
		v.Set("days", strconv.Itoa(f.Days))
	}
	if f.From != "" {
		v.Set("from", f.From)
	}
	if f.To != "" {
		v.Set("to", f.To)
	}
	if f.Namespace != "" {
		v.Set("namespace", f.Namespace)
	}
	if f.Alertname != "" {
		v.Set("alertname", f.Alertname)
	}
	if f.Page > 1 {
		v.Set("page", strconv.Itoa(f.Page))
	}
	if f.PerPage > 0 && f.PerPage != defaultAlertsPerPage {
		v.Set("per_page", strconv.Itoa(f.PerPage))
	}
	return v.Encode()
}

func UniqueNamespaces(alerts []StoredAlert) []string {
	seen := make(map[string]struct{})
	for _, a := range alerts {
		if ns := a.Namespace(); ns != "" {
			seen[ns] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for ns := range seen {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}

func NamespacesForFilter(alerts []StoredAlert, selected string) []string {
	ns := UniqueNamespaces(alerts)
	if selected == "" || selected == NoneNamespaceFilter {
		return ns
	}
	for _, n := range ns {
		if n == selected {
			return ns
		}
	}
	ns = append(ns, selected)
	sort.Strings(ns)
	return ns
}

func AlertsHaveEmptyNamespace(alerts []StoredAlert) bool {
	for _, a := range alerts {
		if a.Namespace() == "" {
			return true
		}
	}
	return false
}

func RankAlertsByNamespace(alerts []StoredAlert) []NamespaceAlertRank {
	counts := make(map[string]int)
	for _, a := range alerts {
		counts[a.Namespace()]++
	}

	ranks := make([]NamespaceAlertRank, 0, len(counts))
	for ns, count := range counts {
		ranks = append(ranks, NamespaceAlertRank{Namespace: ns, Count: count})
	}

	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].Count != ranks[j].Count {
			return ranks[i].Count > ranks[j].Count
		}
		return ranks[i].Namespace < ranks[j].Namespace
	})
	return ranks
}

func ParseAlertFilters(r *http.Request) AlertFilters {
	q := r.URL.Query()
	page, _ := strconv.Atoi(strings.TrimSpace(q.Get("page")))
	if page < 1 {
		page = 1
	}
	days, _ := strconv.Atoi(strings.TrimSpace(q.Get("days")))
	if days < 0 {
		days = 0
	}
	perPage, _ := strconv.Atoi(strings.TrimSpace(q.Get("per_page")))
	if perPage <= 0 {
		perPage = defaultAlertsPerPage
	}
	if perPage > maxAlertsPerPage {
		perPage = maxAlertsPerPage
	}
	return AlertFilters{
		Severity:  strings.TrimSpace(q.Get("severity")),
		Status:    strings.TrimSpace(q.Get("status")),
		Source:    strings.TrimSpace(q.Get("source")),
		DateRange: strings.TrimSpace(q.Get("range")),
		Date:      strings.TrimSpace(q.Get("date")),
		Days:      days,
		From:      strings.TrimSpace(q.Get("from")),
		To:        strings.TrimSpace(q.Get("to")),
		Namespace: strings.TrimSpace(q.Get("namespace")),
		Alertname: strings.TrimSpace(q.Get("alertname")),
		Page:      page,
		PerPage:   perPage,
	}
}

// DaysString returns the Days filter formatted for HTML number inputs.
func (f AlertFilters) DaysString() string {
	if f.Days <= 0 {
		return ""
	}
	return strconv.Itoa(f.Days)
}

// DateRangeSelect returns the value for the Date dropdown (presets or "custom").
func (f AlertFilters) DateRangeSelect() string {
	if f.Days > 0 || f.From != "" || f.To != "" || f.DateRange == "custom" {
		return "custom"
	}
	return f.DateRange
}

// CustomDateOpen reports whether the custom date panel should be visible.
func (f AlertFilters) CustomDateOpen() bool {
	return f.DateRangeSelect() == "custom"
}

// CustomDateMode returns "days" or "calendar" for the custom panel tab state.
func (f AlertFilters) CustomDateMode() string {
	if f.From != "" || f.To != "" {
		return "calendar"
	}
	return "days"
}

// FromDate returns the From filter as YYYY-MM-DDTHH:MM for hidden form fields.
func (f AlertFilters) FromDate() string {
	return formatFilterDateTime(f.From, false)
}

// ToDate returns the To filter as YYYY-MM-DDTHH:MM for hidden form fields.
func (f AlertFilters) ToDate() string {
	return formatFilterDateTime(f.To, true)
}

// FromDateOnly returns the From date part (YYYY-MM-DD) for date inputs.
func (f AlertFilters) FromDateOnly() string {
	return filterDatePart(formatFilterDateTime(f.From, false))
}

// FromTimeOnly returns the From time part (HH:MM, 24-hour) for time inputs.
func (f AlertFilters) FromTimeOnly() string {
	return filterTimePart(formatFilterDateTime(f.From, false))
}

// ToDateOnly returns the To date part (YYYY-MM-DD) for date inputs.
func (f AlertFilters) ToDateOnly() string {
	return filterDatePart(formatFilterDateTime(f.To, true))
}

// ToTimeOnly returns the To time part (HH:MM, 24-hour) for time inputs.
func (f AlertFilters) ToTimeOnly() string {
	return filterTimePart(formatFilterDateTime(f.To, true))
}

func filterDatePart(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}

func filterTimePart(s string) string {
	if len(s) >= 16 {
		return s[11:16]
	}
	return ""
}

func formatFilterDateTime(raw string, end bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	t, ok := parseFilterTime(raw, end)
	if !ok {
		if len(raw) >= 10 {
			day := raw[:10]
			if end {
				return day + "T23:59"
			}
			return day + "T00:00"
		}
		return raw
	}
	return t.UTC().Format("2006-01-02T15:04")
}

func (f AlertFilters) Active() bool {
	return f.Severity != "" || f.Status != "" || f.Source != "" || f.DateRange != "" || f.Date != "" ||
		f.Days > 0 || f.From != "" || f.To != "" ||
		f.Namespace != "" || f.Alertname != ""
}

func (f AlertFilters) Match(a StoredAlert) bool {
	if f.Severity != "" && !strings.EqualFold(a.Labels["severity"], f.Severity) {
		return false
	}
	if f.Status != "" && !strings.EqualFold(a.Status, f.Status) {
		return false
	}
	if f.Source != "" && !strings.EqualFold(a.Source, f.Source) {
		return false
	}
	if f.Namespace != "" {
		if f.Namespace == NoneNamespaceFilter {
			if a.Namespace() != "" {
				return false
			}
		} else if !strings.EqualFold(a.Namespace(), f.Namespace) {
			return false
		}
	}
	if f.Alertname != "" && !strings.Contains(strings.ToLower(a.Labels["alertname"]), strings.ToLower(f.Alertname)) {
		return false
	}
	return f.matchDate(a.UpdatedDisplayTime())
}

func (f AlertFilters) matchDate(received time.Time) bool {
	receivedUTC := received.UTC()

	if from, ok := parseFilterTime(f.From, false); ok {
		if receivedUTC.Before(from) {
			return false
		}
	}
	if to, ok := parseFilterTime(f.To, true); ok {
		if receivedUTC.After(to) {
			return false
		}
	}
	if f.From != "" || f.To != "" {
		return true
	}

	if f.Days > 0 {
		cutoff := time.Now().UTC().Add(-time.Duration(f.Days) * 24 * time.Hour)
		return !receivedUTC.Before(cutoff)
	}

	now := time.Now().UTC()

	switch f.DateRange {
	case "", "all":
		if f.Date == "" {
			return true
		}
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return !receivedUTC.Before(start)
	case "7d":
		return receivedUTC.After(now.Add(-7 * 24 * time.Hour))
	case "14d":
		return receivedUTC.After(now.Add(-14 * 24 * time.Hour))
	case "30d":
		return receivedUTC.After(now.Add(-30 * 24 * time.Hour))
	case "custom":
		return true
	default:
		return true
	}

	if f.Date == "" {
		return true
	}
	day, err := time.ParseInLocation("2006-01-02", f.Date, time.UTC)
	if err != nil {
		return false
	}
	start := day
	end := start.Add(24 * time.Hour)
	return !receivedUTC.Before(start) && receivedUTC.Before(end)
}

// parseFilterTime parses from/to query values as UTC. end=true expands date-only and minute values to inclusive upper bounds.
func parseFilterTime(raw string, end bool) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}

	try := []struct {
		layout string
		adjust func(time.Time) time.Time
	}{
		{time.RFC3339, func(t time.Time) time.Time { return t.UTC() }},
		{"2006-01-02T15:04:05", func(t time.Time) time.Time { return t }},
		{"2006-01-02T15:04", func(t time.Time) time.Time {
			if end {
				return t.Add(time.Minute - time.Nanosecond)
			}
			return t
		}},
		{"2006-01-02", func(t time.Time) time.Time {
			if end {
				return t.Add(24*time.Hour - time.Nanosecond)
			}
			return t
		}},
	}

	for _, item := range try {
		t, err := time.ParseInLocation(item.layout, raw, time.UTC)
		if err == nil {
			return item.adjust(t).UTC(), true
		}
	}
	return time.Time{}, false
}

func FilterAlerts(alerts []StoredAlert, f AlertFilters) []StoredAlert {
	out := make([]StoredAlert, 0, len(alerts))
	for _, a := range alerts {
		if f.Match(a) {
			out = append(out, a)
		}
	}
	sortAlertsByUpdated(out)
	return out
}

func sortAlertsByUpdated(alerts []StoredAlert) {
	sort.SliceStable(alerts, func(i, j int) bool {
		ti := alerts[i].UpdatedDisplayTime()
		tj := alerts[j].UpdatedDisplayTime()
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return alerts[i].ID > alerts[j].ID
	})
}

func PaginateAlerts(alerts []StoredAlert, page, perPage int) (pageAlerts []StoredAlert, totalPages, pageNum int) {
	if perPage <= 0 {
		perPage = defaultAlertsPerPage
	}
	total := len(alerts)
	if total == 0 {
		return nil, 1, 1
	}
	totalPages = (total + perPage - 1) / perPage
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * perPage
	end := start + perPage
	if end > total {
		end = total
	}
	return alerts[start:end], totalPages, page
}
