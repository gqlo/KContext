package kcontext

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var alertsTemplate = template.Must(template.New("alerts").Funcs(template.FuncMap{
	"fmtRelative": FormatRelativeTime,
	"fmtTime":     FormatAlertTime,
	"fmtISO":      FormatTimeISO,
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>KContext — Alerts</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f6f8fa;
      --card: #ffffff;
      --border: #d0d7de;
      --text: #1f2328;
      --muted: #656d76;
      --link: #0969da;
      --critical: #cf222e;
      --warning: #9a6700;
      --info: #0969da;
      --resolved: #1a7f37;
      --firing: #cf222e;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: ui-sans-serif, system-ui, -apple-system, sans-serif;
      background: var(--bg);
      color: var(--text);
      line-height: 1.5;
    }
    header {
      padding: 1.5rem 2rem;
      border-bottom: 1px solid var(--border);
      background: var(--card);
      box-shadow: 0 1px 0 rgba(27,31,36,0.04);
    }
    header h1 { margin: 0; font-size: 1.25rem; font-weight: 600; }
    header h1 a {
      color: inherit;
      text-decoration: none;
    }
    header h1 a:hover { color: var(--link); }
    header p { margin: 0.25rem 0 0; color: var(--muted); font-size: 0.875rem; }
    main { padding: 1.5rem 2rem; max-width: 1200px; margin: 0 auto; }
    .empty { color: var(--muted); padding: 2rem 0; text-align: center; }
    table { width: 100%; border-collapse: collapse; font-size: 0.875rem; background: var(--card); border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
    th, td { text-align: left; padding: 0.75rem 1rem; border-bottom: 1px solid var(--border); vertical-align: top; }
    th { color: var(--muted); font-weight: 600; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.04em; background: #f6f8fa; }
    tbody tr:last-child td { border-bottom: none; }
    tbody tr:hover td { filter: brightness(0.97); }
    .row-firing.row-severity-critical td { background: #ffebe9; }
    .row-firing.row-severity-warning td { background: #fff8c5; }
    .row-firing.row-severity-info td { background: #ddf4ff; }
    .row-firing.row-severity-none td,
    .row-firing.row-severity-unknown td { background: #f6f8fa; }
    .row-resolved td { background: #dafbe1; }
    .row-suppressed td { background: #eaeef2; }
    .badge {
      display: inline-block;
      padding: 0.15rem 0.5rem;
      border-radius: 999px;
      font-size: 0.75rem;
      font-weight: 600;
      text-transform: uppercase;
    }
    .badge-firing { background: #ffebe9; color: var(--critical); border: 1px solid #ff818266; }
    .badge-resolved { background: #dafbe1; color: var(--resolved); border: 1px solid #4ac26b66; }
    .badge-suppressed { background: #eaeef2; color: var(--muted); border: 1px solid var(--border); }
    .severity-critical { color: var(--critical); font-weight: 600; }
    .severity-warning { color: var(--warning); font-weight: 600; }
    .severity-info { color: var(--info); font-weight: 600; }
    .summary { max-width: 28rem; }
    .labels { color: var(--muted); font-size: 0.75rem; margin-top: 0.25rem; }
    time { white-space: nowrap; color: var(--muted); font-size: 0.8125rem; }
    .source { color: var(--muted); font-size: 0.75rem; }
    .filters {
      display: flex;
      flex-wrap: wrap;
      gap: 0.75rem;
      align-items: flex-end;
      margin-bottom: 1.5rem;
      padding: 1rem;
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 8px;
      box-shadow: 0 1px 3px rgba(27,31,36,0.06);
    }
    .field { display: flex; flex-direction: column; gap: 0.25rem; }
    .field label { font-size: 0.75rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em; font-weight: 600; }
    .field input, .field select {
      background: #ffffff;
      color: var(--text);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 0.4rem 0.6rem;
      font-size: 0.875rem;
    }
    .custom-date-panel {
      display: none;
      flex-basis: 100%;
      gap: 0.75rem;
      align-items: flex-end;
      padding: 0.85rem 1rem;
      background: #f6f8fa;
      border: 1px solid var(--border);
      border-radius: 8px;
    }
    .custom-date-panel.open { display: flex; flex-wrap: wrap; }
    .custom-date-tabs {
      display: flex;
      gap: 1rem;
      flex-basis: 100%;
      font-size: 0.875rem;
    }
    .custom-date-tabs label {
      display: flex;
      align-items: center;
      gap: 0.35rem;
      cursor: pointer;
      color: var(--text);
      text-transform: none;
      letter-spacing: normal;
      font-weight: 500;
    }
    .custom-date-body { display: flex; flex-wrap: wrap; gap: 0.75rem; align-items: flex-end; }
    .custom-date-body.hidden { display: none; }
    .actions { display: flex; gap: 0.5rem; align-items: center; }
    button, .btn-link {
      background: #1a7f37;
      color: #fff;
      border: none;
      border-radius: 6px;
      padding: 0.45rem 0.9rem;
      font-size: 0.875rem;
      cursor: pointer;
      text-decoration: none;
    }
    button:hover { background: #2da44e; }
    .btn-link { background: #ffffff; color: var(--muted); border: 1px solid var(--border); }
    .btn-link:hover { background: #f6f8fa; color: var(--text); }
    .runbook { font-size: 0.8125rem; white-space: nowrap; }
    .runbook a { color: var(--link); text-decoration: none; }
    .runbook a:hover { text-decoration: underline; }
    .muted { color: var(--muted); }
    .namespace-link {
      color: var(--link);
      text-decoration: none;
      border-radius: 4px;
      padding: 0.1rem 0.35rem;
    }
    .namespace-link:hover { text-decoration: underline; background: #ddf4ff; }
    .namespace-link.selected {
      background: #ddf4ff;
      font-weight: 600;
      border: 1px solid #54aeff66;
    }
    .alert-link { color: inherit; text-decoration: none; }
    .alert-link:hover { color: var(--link); text-decoration: underline; }
    .pagination {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 1rem;
      margin-top: 1.25rem;
      padding: 0.75rem;
      font-size: 0.875rem;
    }
    .pagination a {
      color: var(--link);
      text-decoration: none;
      padding: 0.35rem 0.75rem;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: #fff;
    }
    .pagination a:hover { background: #f6f8fa; }
    .pagination .disabled {
      color: var(--muted);
      padding: 0.35rem 0.75rem;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: #f6f8fa;
      opacity: 0.6;
    }
    .pagination .page-info { color: var(--muted); }
  </style>
</head>
<body>
  <header>
    <h1><a href="/" title="Clear all filters">KContext</a></h1>
    <p>{{if .Filters.Active}}{{if gt .Filtered 0}}Showing {{.PageStart}}–{{.PageEnd}} of {{.Filtered}} matching alert(s){{else}}No matching alerts{{end}}{{if lt .Filtered .Total}} · {{.Total}} total stored{{end}}{{else}}{{if gt .Filtered 0}}{{.Filtered}} alert(s) stored{{else}}No alerts stored{{end}}{{end}} · newest first</p>
  </header>
  <main>
    <form class="filters" method="get" action="/">
      <div class="field">
        <label for="severity">Severity</label>
        <select id="severity" name="severity">
          <option value="">All</option>
          <option value="critical" {{if eq .Filters.Severity "critical"}}selected{{end}}>Critical</option>
          <option value="warning" {{if eq .Filters.Severity "warning"}}selected{{end}}>Warning</option>
          <option value="info" {{if eq .Filters.Severity "info"}}selected{{end}}>Info</option>
        </select>
      </div>
      <div class="field">
        <label for="status">Status</label>
        <select id="status" name="status">
          <option value="">All</option>
          <option value="firing" {{if eq .Filters.Status "firing"}}selected{{end}}>Firing</option>
          <option value="resolved" {{if eq .Filters.Status "resolved"}}selected{{end}}>Resolved</option>
          <option value="suppressed" {{if eq .Filters.Status "suppressed"}}selected{{end}}>Suppressed</option>
        </select>
      </div>
      <div class="field">
        <label for="source">Source</label>
        <select id="source" name="source">
          <option value="">All</option>
          <option value="poll" {{if eq .Filters.Source "poll"}}selected{{end}}>Poll</option>
          <option value="webhook" {{if eq .Filters.Source "webhook"}}selected{{end}}>Webhook</option>
        </select>
      </div>
      <div class="field">
        <label for="range">Date</label>
        <select id="range" name="range">
          <option value="">All time</option>
          <option value="today" {{if eq .Filters.DateRange "today"}}selected{{end}}>Today</option>
          <option value="7d" {{if eq .Filters.DateRange "7d"}}selected{{end}}>Past 7 days</option>
          <option value="14d" {{if eq .Filters.DateRange "14d"}}selected{{end}}>Past 14 days</option>
          <option value="30d" {{if eq .Filters.DateRange "30d"}}selected{{end}}>Past 30 days</option>
          <option value="custom" {{if eq .Filters.DateRangeSelect "custom"}}selected{{end}}>Custom…</option>
        </select>
      </div>
      <div id="custom-date-panel" class="custom-date-panel{{if .Filters.CustomDateOpen}} open{{end}}">
        <div class="custom-date-tabs">
          <label><input type="radio" name="custom_mode" value="days" {{if eq .Filters.CustomDateMode "days"}}checked{{end}}> Past N days</label>
          <label><input type="radio" name="custom_mode" value="calendar" {{if eq .Filters.CustomDateMode "calendar"}}checked{{end}}> Calendar range</label>
        </div>
        <div id="custom-days-body" class="custom-date-body{{if eq .Filters.CustomDateMode "calendar"}} hidden{{end}}">
          <div class="field">
            <label for="days">Days</label>
            <input id="days" type="number" name="days" min="1" max="3650" value="{{.Filters.DaysString}}" placeholder="e.g. 3">
          </div>
        </div>
        <div id="custom-calendar-body" class="custom-date-body{{if eq .Filters.CustomDateMode "days"}} hidden{{end}}">
          <div class="field">
            <label for="from">From</label>
            <input id="from" type="date" name="from" value="{{.Filters.FromDate}}">
          </div>
          <div class="field">
            <label for="to">To</label>
            <input id="to" type="date" name="to" value="{{.Filters.ToDate}}">
          </div>
        </div>
        <button type="submit" id="apply-dates">Apply</button>
      </div>
      <div class="field">
        <label for="namespace">Namespace</label>
        <select id="namespace" name="namespace">
          <option value="">All</option>
          {{range .Namespaces}}
          <option value="{{.}}" {{if eq $.Filters.Namespace .}}selected{{end}}>{{.}}</option>
          {{end}}
        </select>
      </div>
      <div class="field">
        <label for="alertname">Alert name</label>
        <input id="alertname" type="text" name="alertname" value="{{.Filters.Alertname}}" placeholder="HighCPU" title="Press Enter to apply">
      </div>
      <div class="actions">
        <a class="btn-link" href="/">Clear all</a>
      </div>
    </form>
    {{if .Alerts}}
    <table>
      <thead>
        <tr>
          <th>Received</th>
          <th>Status</th>
          <th>Alert</th>
          <th>Namespace</th>
          <th>Severity</th>
          <th>Source</th>
          <th>Summary</th>
          <th>Runbook</th>
        </tr>
      </thead>
      <tbody>
        {{range .Alerts}}
        <tr class="{{.RowClass}}">
          <td><time class="relative-time" datetime="{{fmtISO .ReceivedAt}}" title="{{fmtTime .ReceivedAt}}">{{fmtRelative .ReceivedAt}}</time></td>
          <td><span class="badge badge-{{.Status}}">{{.Status}}</span></td>
          <td>
            <a class="alert-link" href="{{$.AlertDetailLink .ID}}"><strong>{{index .Labels "alertname"}}</strong></a>
            {{if .Pod}}<div class="labels">pod: {{.Pod}}</div>{{end}}
          </td>
          <td>{{if .Namespace}}<a class="namespace-link {{if eq .Namespace $.Filters.Namespace}}selected{{end}}" href="{{$.NamespaceLink .Namespace}}">{{.Namespace}}</a>{{else}}<span class="muted">—</span>{{end}}</td>
          <td class="severity-{{index .Labels "severity"}}">{{index .Labels "severity"}}</td>
          <td class="source">{{.Source}}</td>
          <td class="summary">{{index .Annotations "summary"}}</td>
          <td class="runbook">{{if .RunbookURL}}<a href="{{.RunbookURL}}" target="_blank" rel="noopener noreferrer">Open</a>{{else}}<span class="muted">—</span>{{end}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{if gt .TotalPages 1}}
    <nav class="pagination">
      {{if .PrevPageLink}}<a href="{{.PrevPageLink}}">← Prev</a>{{else}}<span class="disabled">← Prev</span>{{end}}
      <span class="page-info">Page {{.Page}} of {{.TotalPages}}</span>
      {{if .NextPageLink}}<a href="{{.NextPageLink}}">Next →</a>{{else}}<span class="disabled">Next →</span>{{end}}
    </nav>
    {{end}}
    {{else}}
    <p class="empty">{{if .Filters.Active}}No alerts match the current filters.{{else}}No alerts yet. Enable Alertmanager polling or point Alertmanager at <code>/webhook</code>.{{end}}</p>
    {{end}}
  </main>
  <script>
    (function () {
      var form = document.querySelector('form.filters');
      if (!form) return;
      var range = document.getElementById('range');
      var panel = document.getElementById('custom-date-panel');
      var daysBody = document.getElementById('custom-days-body');
      var calendarBody = document.getElementById('custom-calendar-body');
      var days = document.getElementById('days');
      var from = document.getElementById('from');
      var to = document.getElementById('to');
      var applyDates = document.getElementById('apply-dates');
      var modeRadios = form.querySelectorAll('input[name="custom_mode"]');

      function isCustomRange() {
        return range && range.value === 'custom';
      }

      function showPanel(show) {
        if (!panel) return;
        panel.classList.toggle('open', show);
      }

      function customMode() {
        var checked = form.querySelector('input[name="custom_mode"]:checked');
        return checked ? checked.value : 'days';
      }

      function syncModePanels() {
        var mode = customMode();
        if (daysBody) daysBody.classList.toggle('hidden', mode !== 'days');
        if (calendarBody) calendarBody.classList.toggle('hidden', mode !== 'calendar');
      }

      function clearCustomInputs() {
        if (days) days.value = '';
        if (from) from.value = '';
        if (to) to.value = '';
      }

      function prepareCustomSubmit() {
        if (range) range.value = 'custom';
        var mode = customMode();
        if (mode === 'days') {
          if (from) { from.value = ''; from.disabled = true; }
          if (to) { to.value = ''; to.disabled = true; }
          if (days) days.disabled = false;
        } else {
          if (days) { days.value = ''; days.disabled = true; }
          if (from) from.disabled = false;
          if (to) to.disabled = false;
        }
      }

      function enableCustomInputs() {
        if (days) days.disabled = false;
        if (from) from.disabled = false;
        if (to) to.disabled = false;
      }

      form.querySelectorAll('select').forEach(function (el) {
        el.addEventListener('change', function () {
          if (el === range) {
            if (el.value === 'custom') {
              showPanel(true);
              syncModePanels();
              enableCustomInputs();
              return;
            }
            showPanel(false);
            clearCustomInputs();
          }
          form.submit();
        });
      });

      modeRadios.forEach(function (radio) {
        radio.addEventListener('change', syncModePanels);
      });

      if (applyDates) {
        applyDates.addEventListener('click', function (e) {
          e.preventDefault();
          prepareCustomSubmit();
          form.submit();
        });
      }

      if (isCustomRange()) {
        showPanel(true);
        syncModePanels();
      }

      var alertname = document.getElementById('alertname');
      if (alertname) {
        alertname.addEventListener('keydown', function (e) {
          if (e.key === 'Enter') { e.preventDefault(); form.submit(); }
        });
      }
    })();
` + RelativeTimeRefreshJS + `
  </script>
</body>
</html>`))

type Server struct {
	store      *AlertStore
	slackToken string
	channelID  string

	mu          sync.Mutex
	dailyThread map[string]string
}

// NewServer constructs an HTTP handler bundle for the dashboard and webhook.
func NewServer(store *AlertStore, slackToken, channelID string) *Server {
	return &Server{
		store:       store,
		slackToken:  slackToken,
		channelID:   channelID,
		dailyThread: map[string]string{},
	}
}

// Store returns the alert store used by this server.
func (s *Server) Store() *AlertStore {
	return s.store
}

type alertPayload struct {
	Alerts []Alert `json:"alerts"`
}

func (s *Server) HandleAlertsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filters := ParseAlertFilters(r)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	all, err := s.store.List(ctx, 500)
	if err != nil {
		log.Printf("list alerts: %v", err)
		http.Error(w, "failed to load alerts", http.StatusInternalServerError)
		return
	}

	alerts := FilterAlerts(all, filters)
	pageAlerts, totalPages, page := PaginateAlerts(alerts, filters.Page)
	filters.Page = page

	pageStart, pageEnd := 0, 0
	if len(pageAlerts) > 0 {
		pageStart = (page-1)*alertsPerPage + 1
		pageEnd = pageStart + len(pageAlerts) - 1
	}

	var buf bytes.Buffer
	if err := alertsTemplate.Execute(&buf, AlertsPageData{
		Alerts:     pageAlerts,
		Count:      len(pageAlerts),
		Filtered:   len(alerts),
		Total:      len(all),
		Filters:    filters,
		Namespaces: NamespacesForFilter(all, filters.Namespace),
		Page:       page,
		TotalPages: totalPages,
		PageStart:  pageStart,
		PageEnd:    pageEnd,
	}); err != nil {
		log.Printf("render alerts: %v", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

func (s *Server) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload alertPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	for _, alert := range payload.Alerts {
		if err := s.store.Save(ctx, alert); err != nil {
			log.Printf("store alert: %v", err)
		}
	}

	if s.SlackEnabled() {
		s.postAlertsToSlack(payload.Alerts)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) SlackEnabled() bool {
	return s.slackToken != "" && s.channelID != ""
}

func (s *Server) postAlertsToSlack(alerts []Alert) {
	date := time.Now().Format("2006-01-02")
	threadTs, err := s.getOrCreateThread(date)
	if err != nil {
		log.Printf("slack thread: %v", err)
		return
	}

	for _, alert := range alerts {
		text := fmt.Sprintf("*%s* — %s\nSeverity: `%s`",
			alert.Labels["alertname"],
			alert.Annotations["summary"],
			alert.Labels["severity"],
		)
		if u := strings.TrimSpace(alert.Annotations["runbook_url"]); strings.HasPrefix(u, "http") {
			text += fmt.Sprintf("\nRunbook: %s", u)
		}

		if _, err := postSlack(s.slackToken, s.channelID, map[string]any{
			"channel":   s.channelID,
			"text":      text,
			"thread_ts": threadTs,
		}); err != nil {
			log.Printf("slack post alert: %v", err)
		}
	}
}

func (s *Server) getOrCreateThread(date string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ts, ok := s.dailyThread[date]; ok {
		return ts, nil
	}

	ts, err := postSlack(s.slackToken, s.channelID, map[string]any{
		"channel": s.channelID,
		"text":    fmt.Sprintf(":bell: *OpenShift Alert Summary — %s*", date),
	})
	if err != nil {
		return "", err
	}

	s.dailyThread[date] = ts
	return ts, nil
}
