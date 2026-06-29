package main

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const alertsPerPage = 50

type AlertFilters struct {
	Severity  string
	Status    string
	Source    string
	DateRange string
	Date      string // legacy single-day filter (YYYY-MM-DD)
	Namespace string
	Alertname string
	Page      int
}

type alertsPageData struct {
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
}

func (d alertsPageData) PageLink(page int) string {
	f := d.Filters
	f.Page = page
	return "/?" + f.Encode()
}

func (d alertsPageData) PrevPageLink() string {
	if d.Page <= 1 {
		return ""
	}
	return d.PageLink(d.Page - 1)
}

func (d alertsPageData) NextPageLink() string {
	if d.Page >= d.TotalPages {
		return ""
	}
	return d.PageLink(d.Page + 1)
}

func (d alertsPageData) NamespaceLink(ns string) string {
	f := d.Filters
	f.Namespace = ns
	f.Page = 1
	return "/?" + f.Encode()
}

func (d alertsPageData) AlertDetailLink(id string) string {
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
	if f.DateRange != "" {
		v.Set("range", f.DateRange)
	}
	if f.Date != "" {
		v.Set("date", f.Date)
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
	return v.Encode()
}

func uniqueNamespaces(alerts []StoredAlert) []string {
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

func namespacesForFilter(alerts []StoredAlert, selected string) []string {
	ns := uniqueNamespaces(alerts)
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

func parseAlertFilters(r *http.Request) AlertFilters {
	q := r.URL.Query()
	page, _ := strconv.Atoi(strings.TrimSpace(q.Get("page")))
	if page < 1 {
		page = 1
	}
	return AlertFilters{
		Severity:  strings.TrimSpace(q.Get("severity")),
		Status:    strings.TrimSpace(q.Get("status")),
		Source:    strings.TrimSpace(q.Get("source")),
		DateRange: strings.TrimSpace(q.Get("range")),
		Date:      strings.TrimSpace(q.Get("date")),
		Namespace: strings.TrimSpace(q.Get("namespace")),
		Alertname: strings.TrimSpace(q.Get("alertname")),
		Page:      page,
	}
}

func (f AlertFilters) Active() bool {
	return f.Severity != "" || f.Status != "" || f.Source != "" || f.DateRange != "" || f.Date != "" ||
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
	return f.matchDate(a.ReceivedAt)
}

func (f AlertFilters) matchDate(received time.Time) bool {
	loc := time.Now().Location()
	now := time.Now().In(loc)
	receivedLocal := received.In(loc)

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

func filterAlerts(alerts []StoredAlert, f AlertFilters) []StoredAlert {
	out := make([]StoredAlert, 0, len(alerts))
	for _, a := range alerts {
		if f.Match(a) {
			out = append(out, a)
		}
	}
	return out
}

func paginateAlerts(alerts []StoredAlert, page int) (pageAlerts []StoredAlert, totalPages, pageNum int) {
	total := len(alerts)
	if total == 0 {
		return nil, 1, 1
	}
	totalPages = (total + alertsPerPage - 1) / alertsPerPage
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * alertsPerPage
	end := start + alertsPerPage
	if end > total {
		end = total
	}
	return alerts[start:end], totalPages, page
}
