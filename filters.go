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

type AlertFilters struct {
	Severity  string
	Status    string
	Source    string
	DateRange string
	Date      string // legacy single-day filter (YYYY-MM-DD)
	Days      int    // past N days (overrides DateRange presets when > 0)
	From      string // start of time window (datetime-local, date, or RFC3339)
	To        string // end of time window
	Namespace string
	Alertname string
	Page      int
	PerPage   int
}

type AlertsPageData struct {
	Alerts     []StoredAlert
	Count      int
	Filtered   int
	Total      int
	Filters    AlertFilters
	Namespaces []string
	Page       int
	TotalPages int
	PageStart  int
	PageEnd    int
	Cluster    ClusterMeta
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

func (d AlertsPageData) NextPageLink() string {
	if d.Page >= d.TotalPages {
		return ""
	}
	return d.PageLink(d.Page + 1)
}

func (d AlertsPageData) NamespaceLink(ns string) string {
	f := d.Filters
	f.Namespace = ns
	f.Page = 1
	return "/?" + f.Encode()
}

func (d AlertsPageData) AlertDetailLink(id string) string {
	return "/alert?id=" + url.QueryEscape(id) + "&" + d.Filters.Encode()
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
	if selected == "" {
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

// FromDate returns the From filter as YYYY-MM-DD for date inputs.
func (f AlertFilters) FromDate() string {
	return dateOnlyValue(f.From)
}

// ToDate returns the To filter as YYYY-MM-DD for date inputs.
func (f AlertFilters) ToDate() string {
	return dateOnlyValue(f.To)
}

func dateOnlyValue(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
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
	if f.Namespace != "" && !strings.EqualFold(a.Namespace(), f.Namespace) {
		return false
	}
	if f.Alertname != "" && !strings.Contains(strings.ToLower(a.Labels["alertname"]), strings.ToLower(f.Alertname)) {
		return false
	}
	return f.matchDate(a.UpdatedDisplayTime())
}

func (f AlertFilters) matchDate(received time.Time) bool {
	loc := time.Now().Location()
	receivedLocal := received.In(loc)

	if from, ok := parseFilterTime(f.From, false); ok {
		if receivedLocal.Before(from) {
			return false
		}
	}
	if to, ok := parseFilterTime(f.To, true); ok {
		if receivedLocal.After(to) {
			return false
		}
	}
	if f.From != "" || f.To != "" {
		return true
	}

	if f.Days > 0 {
		cutoff := time.Now().Add(-time.Duration(f.Days) * 24 * time.Hour)
		return !received.Before(cutoff)
	}

	now := time.Now().In(loc)

	switch f.DateRange {
	case "", "all":
		if f.Date == "" {
			return true
		}
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		return !receivedLocal.Before(start)
	case "7d":
		return received.After(time.Now().Add(-7 * 24 * time.Hour))
	case "14d":
		return received.After(time.Now().Add(-14 * 24 * time.Hour))
	case "30d":
		return received.After(time.Now().Add(-30 * 24 * time.Hour))
	case "custom":
		return true
	default:
		return true
	}

	if f.Date == "" {
		return true
	}
	day, err := time.ParseInLocation("2006-01-02", f.Date, loc)
	if err != nil {
		return false
	}
	start := day
	end := start.Add(24 * time.Hour)
	return !receivedLocal.Before(start) && receivedLocal.Before(end)
}

// parseFilterTime parses from/to query values. end=true expands date-only and minute values to inclusive upper bounds.
func parseFilterTime(raw string, end bool) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}

	loc := time.Now().Location()
	try := []struct {
		layout string
		adjust func(time.Time) time.Time
	}{
		{time.RFC3339, func(t time.Time) time.Time { return t }},
		{"2006-01-02T15:04:05", func(t time.Time) time.Time {
			if end {
				return t
			}
			return t
		}},
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
		t, err := time.ParseInLocation(item.layout, raw, loc)
		if err == nil {
			return item.adjust(t), true
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
