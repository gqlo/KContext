package kcontext

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

var alertDetailTemplate = template.Must(template.New("alert-detail").Funcs(template.FuncMap{
	"fmtTime":     FormatAlertTime,
	"fmtRelative": FormatRelativeTime,
	"fmtISO":      FormatTimeISO,
	"hasPrefix":   strings.HasPrefix,
	"headerIntro": headerIntro,
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{index .Alert.Labels "alertname"}} — KContext</title>
  <style>
    :root {
      --bg: #f6f8fa;
      --card: #ffffff;
      --border: #d0d7de;
      --text: #1f2328;
      --muted: #656d76;
      --link: #0969da;
      --critical: #cf222e;
      --warning: #9a6700;
      --resolved: #1a7f37;
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
      padding: 1.25rem 2rem;
      border-bottom: 1px solid var(--border);
      background: var(--card);
    }
    header h1 { margin: 0; font-size: 1.25rem; }
    header h1 a { color: inherit; text-decoration: none; }
    header h1 a:hover { color: var(--link); }
    header .subtitle { margin: 0.15rem 0 0; font-size: 0.9375rem; font-weight: 600; color: var(--text); }
    header .about,
    header .does,
    header .repo,
    header .meta { margin: 0.35rem 0 0; color: var(--muted); font-size: 0.875rem; max-width: 72rem; line-height: 1.45; }
    header .about strong,
    header .does strong { color: var(--text); font-weight: 600; }
    header .repo a { color: var(--link); text-decoration: none; }
    header .repo a:hover { text-decoration: underline; }
    main { padding: 1rem 1.5rem 1.5rem; width: 100%; max-width: none; margin: 0; }
    .back { display: inline-block; margin-bottom: 1rem; color: var(--link); text-decoration: none; font-size: 0.875rem; }
    .back:hover { text-decoration: underline; }
    .panel {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 8px;
      margin-bottom: 1rem;
      overflow: hidden;
    }
    .panel h2 {
      margin: 0;
      padding: 0.75rem 1rem;
      font-size: 0.75rem;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      color: var(--muted);
      background: #f6f8fa;
      border-bottom: 1px solid var(--border);
    }
    table { width: 100%; border-collapse: collapse; font-size: 0.875rem; }
    th, td { text-align: left; padding: 0.6rem 1rem; border-bottom: 1px solid var(--border); vertical-align: top; }
    tr:last-child td { border-bottom: none; }
    th { width: 11rem; color: var(--muted); font-weight: 600; }
    td a { color: var(--link); word-break: break-all; }
    .badge {
      display: inline-block;
      padding: 0.15rem 0.5rem;
      border-radius: 999px;
      font-size: 0.75rem;
      font-weight: 600;
      text-transform: uppercase;
    }
    .badge-firing { background: #ffebe9; color: var(--critical); }
    .badge-resolved { background: #dafbe1; color: var(--resolved); }
    .badge-suppressed { background: #eaeef2; color: var(--muted); }
    .summary-block {
      padding: 1rem;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .mono { font-family: ui-monospace, monospace; font-size: 0.8125rem; word-break: break-all; }
  </style>
</head>
<body>
  <header>
    <h1><a href="/" title="Clear all filters">KContext</a></h1>
    {{headerIntro}}
    <p class="meta">Alert detail</p>
  </header>
  <main>
    <a class="back" href="{{.BackLink}}">← Back to alerts</a>

    <div class="panel">
      <h2>Overview</h2>
      <table>
        <tr><th>Alert name</th><td><strong>{{index .Alert.Labels "alertname"}}</strong></td></tr>
        <tr><th>Status</th><td><span class="badge badge-{{.Alert.Status}}">{{.Alert.Status}}</span></td></tr>
        <tr><th>Severity</th><td>{{index .Alert.Labels "severity"}}</td></tr>
        <tr><th>Namespace</th><td>{{.Alert.Namespace}}</td></tr>
        <tr><th>Source</th><td>{{.Alert.Source}}</td></tr>
        <tr><th>Updated at</th><td><time class="relative-time" datetime="{{fmtISO .Alert.UpdatedDisplayTime}}" title="{{fmtTime .Alert.UpdatedDisplayTime}}">{{fmtRelative .Alert.UpdatedDisplayTime}}</time></td></tr>
        <tr><th>Received by KContext</th><td>{{fmtTime .Alert.ReceivedAt}}</td></tr>
        <tr><th>Starts at</th><td>{{fmtTime .Alert.StartsAt}}</td></tr>
        <tr><th>Ends at</th><td>{{fmtTime .Alert.EndsAt}}</td></tr>
        <tr><th>Fingerprint</th><td class="mono">{{.Alert.Fingerprint}}</td></tr>
        <tr><th>ID</th><td class="mono">{{.Alert.ID}}</td></tr>
      </table>
    </div>

    {{if index .Alert.Annotations "summary"}}
    <div class="panel">
      <h2>Summary</h2>
      <div class="summary-block">{{index .Alert.Annotations "summary"}}</div>
    </div>
    {{end}}

    {{if index .Alert.Annotations "description"}}
    <div class="panel">
      <h2>Description</h2>
      <div class="summary-block">{{index .Alert.Annotations "description"}}</div>
    </div>
    {{end}}

    <div class="panel">
      <h2>Links</h2>
      <table>
        <tr><th>Runbook</th><td>{{if .Alert.RunbookURL}}<a href="{{.Alert.RunbookURL}}" target="_blank" rel="noopener noreferrer">{{.Alert.RunbookURL}}</a>{{else}}—{{end}}</td></tr>
        <tr><th>Generator URL</th><td>{{if .Alert.GeneratorURL}}<a href="{{.Alert.GeneratorURL}}" target="_blank" rel="noopener noreferrer">{{.Alert.GeneratorURL}}</a>{{else}}—{{end}}</td></tr>
      </table>
    </div>

    <div class="panel">
      <h2>Labels</h2>
      <table>
        {{range .LabelKeys}}
        <tr><th>{{.}}</th><td class="mono">{{index $.Alert.Labels .}}</td></tr>
        {{else}}
        <tr><td colspan="2">No labels</td></tr>
        {{end}}
      </table>
    </div>

    <div class="panel">
      <h2>Annotations</h2>
      <table>
        {{range .AnnotationKeys}}
        <tr><th>{{.}}</th><td>{{if or (eq . "runbook_url") (eq . "runbook") (hasPrefix (index $.Alert.Annotations .) "http")}}<a href="{{index $.Alert.Annotations .}}" target="_blank" rel="noopener noreferrer">{{index $.Alert.Annotations .}}</a>{{else}}<span class="summary-block" style="padding:0;display:block;">{{index $.Alert.Annotations .}}</span>{{end}}</td></tr>
        {{else}}
        <tr><td colspan="2">No annotations</td></tr>
        {{end}}
      </table>
    </div>
  </main>
  <script>` + RelativeTimeRefreshJS + `</script>
</body>
</html>`))

type alertDetailData struct {
	Alert          StoredAlert
	BackLink       string
	LabelKeys      []string
	AnnotationKeys []string
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Server) HandleAlertDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var alert *StoredAlert
	if s.snapshot != nil {
		alert = s.snapshot.getByID(id)
	}
	if alert == nil {
		var err error
		alert, err = s.store.Get(ctx, id)
		if err != nil {
			log.Printf("get alert %s: %v", id, err)
			http.Error(w, "failed to load alert", http.StatusInternalServerError)
			return
		}
	}
	if alert == nil {
		http.NotFound(w, r)
		return
	}

	filters := ParseAlertFilters(r)
	back := "/"
	if q := filters.Encode(); q != "" {
		back = "/?" + q
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := alertDetailTemplate.Execute(w, alertDetailData{
		Alert:          *alert,
		BackLink:       back,
		LabelKeys:      sortedKeys(alert.Labels),
		AnnotationKeys: sortedKeys(alert.Annotations),
	}); err != nil {
		log.Printf("render alert detail: %v", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}
