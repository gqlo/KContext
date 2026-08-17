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
	"headerIntro": headerIntro,
	"utcClock":    utcClock,
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
      padding: 1rem 1.5rem;
      border-bottom: 1px solid var(--border);
      background: var(--card);
      box-shadow: 0 1px 0 rgba(27,31,36,0.04);
    }
    .header-top {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 1rem;
      flex-wrap: wrap;
    }
    header h1 { margin: 0; font-size: 1.25rem; font-weight: 600; }
    header h1 a {
      color: inherit;
      text-decoration: none;
    }
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
    .utc-clock {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      padding: 0.35rem 0.75rem;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: #f6f8fa;
      font-size: 0.8125rem;
      white-space: nowrap;
    }
    .utc-clock-label {
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      color: var(--muted);
      font-size: 0.75rem;
    }
    .utc-clock-time {
      font-family: ui-monospace, monospace;
      font-variant-numeric: tabular-nums;
      color: var(--text);
      font-weight: 600;
    }
    .page-body {
      display: flex;
      gap: 1.25rem;
      align-items: stretch;
      width: 100%;
      min-height: calc(100vh - 4.5rem);
      padding: 1rem 1.5rem 1.5rem;
    }
    .sidebar {
      width: 280px;
      flex-shrink: 0;
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 1rem 1.15rem;
      position: sticky;
      top: 1rem;
      align-self: flex-start;
      max-height: calc(100vh - 2rem);
      overflow-y: auto;
      box-shadow: 0 1px 3px rgba(27,31,36,0.06);
    }
    .sidebar h2 {
      margin: 0 0 0.75rem;
      font-size: 0.8125rem;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      color: var(--muted);
    }
    .meta-list { margin: 0; }
    .meta-list dt {
      font-size: 0.75rem;
      color: var(--muted);
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      margin-top: 0.85rem;
    }
    .meta-list dt:first-child { margin-top: 0; }
    .meta-list dd { margin: 0.2rem 0 0; font-size: 0.9375rem; font-weight: 600; }
    .sidebar-section { margin-top: 1.25rem; }
    .sidebar-section h2 {
      margin: 0 0 0.65rem;
      font-size: 0.8125rem;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      color: var(--muted);
    }
    .sidebar-table-wrap { max-height: none; }
    .sidebar-table {
      width: 100%;
      border-collapse: collapse;
      font-size: 0.8125rem;
    }
    .sidebar-table th,
    .sidebar-table td {
      padding: 0.4rem 0;
      border-bottom: 1px solid var(--border);
      vertical-align: top;
    }
    .sidebar-table th {
      color: var(--muted);
      font-size: 0.6875rem;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      text-align: left;
    }
    .sidebar-table th.count,
    .sidebar-table td.count { text-align: right; white-space: nowrap; }
    .sidebar-table tbody tr:last-child td { border-bottom: none; }
    .sidebar-table a { color: var(--link); text-decoration: none; }
    .sidebar-table a:hover { text-decoration: underline; }
    .main-col { flex: 1; min-width: 0; width: 100%; }
    .empty { color: var(--muted); padding: 2rem 0; text-align: center; }
    .alerts-table-wrap { width: 100%; overflow-x: auto; }
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
    .summary { min-width: 12rem; }
    .labels { color: var(--muted); font-size: 0.75rem; margin-top: 0.25rem; }
    time { white-space: nowrap; color: var(--muted); font-size: 0.8125rem; }
    .source { color: var(--muted); font-size: 0.75rem; }
    .filters {
      display: flex;
      flex-wrap: wrap;
      gap: 0.75rem;
      align-items: flex-end;
      width: 100%;
      margin-bottom: 1.25rem;
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
    .datetime-range-inputs {
      display: flex;
      gap: 0.35rem;
      align-items: center;
      flex-wrap: wrap;
    }
    .field input.time-24h {
      width: 4.75rem;
      min-width: 4.75rem;
      font-family: ui-monospace, monospace;
      font-variant-numeric: tabular-nums;
      text-align: center;
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
      margin: 1rem 0;
      padding: 0.75rem;
      font-size: 0.875rem;
      flex-wrap: wrap;
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
    <div class="header-top">
      <h1><a href="/" title="Clear all filters">KContext</a></h1>
      {{utcClock}}
    </div>
    {{headerIntro}}
    <p class="meta">{{if gt .Filtered 0}}Showing {{.PageStart}}–{{.PageEnd}} of {{.Filtered}} alert(s){{if lt .Filtered .Total}} · {{.Total}} total stored{{end}}{{else}}No alerts match the current filters{{end}} · newest first</p>
  </header>
  <div class="page-body">
    <aside class="sidebar">
      <h2>Cluster</h2>
      <dl class="meta-list">
        <dt>Nodes</dt>
        <dd>{{if .Cluster.NodesDisplay}}{{.Cluster.NodesDisplay}}{{else}}—{{end}}</dd>
        <dt>OCP version</dt>
        <dd>{{if .Cluster.OCPVersion}}{{.Cluster.OCPVersion}}{{else}}—{{end}}</dd>
        <dt>CNV version</dt>
        <dd>{{if .Cluster.CNVVersion}}{{.Cluster.CNVVersion}}{{else}}Not installed{{end}}</dd>
        <dt>ODF version</dt>
        <dd>{{if .Cluster.ODFVersion}}{{.Cluster.ODFVersion}}{{else}}Not installed{{end}}</dd>
      </dl>
      {{if .NamespaceRanks}}
      <div class="sidebar-section">
        <h2>Alerts by namespace</h2>
        <div class="sidebar-table-wrap">
          <table class="sidebar-table">
            <thead>
              <tr>
                <th>Namespace</th>
                <th class="count">Alerts</th>
              </tr>
            </thead>
            <tbody>
              {{range .NamespaceRanks}}
              <tr>
                <td>{{if .Namespace}}<a class="namespace-link {{if eq $.Filters.Namespace .Namespace}}selected{{end}}" href="{{$.NamespaceLink .Namespace}}">{{.Namespace}}</a>{{else}}<a class="namespace-link {{if $.Filters.NamespaceIsNone}}selected{{end}}" href="{{$.NoneNamespaceLink}}">(none)</a>{{end}}</td>
                <td class="count">{{.Count}}</td>
              </tr>
              {{end}}
            </tbody>
          </table>
        </div>
      </div>
      {{end}}
    </aside>
    <div class="main-col">
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
          <option value="today" {{if eq .Filters.DateRange "today"}}selected{{end}}>Today (UTC)</option>
          <option value="7d" {{if eq .Filters.DateRange "7d"}}selected{{end}}>Past 7 days</option>
          <option value="14d" {{if eq .Filters.DateRange "14d"}}selected{{end}}>Past 14 days</option>
          <option value="30d" {{if eq .Filters.DateRange "30d"}}selected{{end}}>Past 30 days</option>
          <option value="custom" {{if eq .Filters.DateRangeSelect "custom"}}selected{{end}}>Custom…</option>
        </select>
      </div>
      <div id="custom-date-panel" class="custom-date-panel{{if .Filters.CustomDateOpen}} open{{end}}">
        <div class="custom-date-tabs">
          <label><input type="radio" name="custom_mode" value="days" {{if eq .Filters.CustomDateMode "days"}}checked{{end}}> Past N days</label>
          <label><input type="radio" name="custom_mode" value="calendar" {{if eq .Filters.CustomDateMode "calendar"}}checked{{end}}> Date & time range</label>
        </div>
        <div id="custom-days-body" class="custom-date-body{{if eq .Filters.CustomDateMode "calendar"}} hidden{{end}}">
          <div class="field">
            <label for="days">Days</label>
            <input id="days" type="number" name="days" min="1" max="3650" value="{{.Filters.DaysString}}" placeholder="e.g. 3">
          </div>
        </div>
        <div id="custom-calendar-body" class="custom-date-body{{if eq .Filters.CustomDateMode "days"}} hidden{{end}}">
          <div class="field">
            <label for="from-date">From (UTC)</label>
            <div class="datetime-range-inputs">
              <input id="from-date" type="date" value="{{.Filters.FromDateOnly}}">
              <input id="from-time" type="text" class="time-24h" value="{{.Filters.FromTimeOnly}}" placeholder="HH:MM" maxlength="5" pattern="([01][0-9]|2[0-3]):[0-5][0-9]" title="24-hour time (HH:MM)" autocomplete="off">
              <input id="from" type="hidden" name="from" value="{{.Filters.FromDate}}">
            </div>
          </div>
          <div class="field">
            <label for="to-date">To (UTC)</label>
            <div class="datetime-range-inputs">
              <input id="to-date" type="date" value="{{.Filters.ToDateOnly}}">
              <input id="to-time" type="text" class="time-24h" value="{{.Filters.ToTimeOnly}}" placeholder="HH:MM" maxlength="5" pattern="([01][0-9]|2[0-3]):[0-5][0-9]" title="24-hour time (HH:MM)" autocomplete="off">
              <input id="to" type="hidden" name="to" value="{{.Filters.ToDate}}">
            </div>
          </div>
        </div>
        <button type="submit" id="apply-dates">Apply</button>
      </div>
      <div class="field">
        <label for="namespace">Namespace</label>
        <select id="namespace" name="namespace">
          <option value="">All</option>
          {{if .HasEmptyNamespaceAlerts}}
          <option value="{{$.Filters.NoneNamespaceFilterValue}}" {{if $.Filters.NamespaceIsNone}}selected{{end}}>(none)</option>
          {{end}}
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
    {{if gt .TotalPages 1}}
    <nav class="pagination">
      {{if .FirstPageLink}}<a href="{{.FirstPageLink}}">First</a>{{else}}<span class="disabled">First</span>{{end}}
      {{if .PrevPageLink}}<a href="{{.PrevPageLink}}">← Prev</a>{{else}}<span class="disabled">← Prev</span>{{end}}
      <span class="page-info">Page {{.Page}} of {{.TotalPages}}</span>
      {{if .NextPageLink}}<a href="{{.NextPageLink}}">Next →</a>{{else}}<span class="disabled">Next →</span>{{end}}
      {{if .LastPageLink}}<a href="{{.LastPageLink}}">Last</a>{{else}}<span class="disabled">Last</span>{{end}}
    </nav>
    {{end}}
    <div class="alerts-table-wrap">
    <table>
      <thead>
        <tr>
          <th>Updated at</th>
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
          <td><time class="relative-time" datetime="{{fmtISO .UpdatedDisplayTime}}" title="{{fmtTime .UpdatedDisplayTime}}">{{fmtRelative .UpdatedDisplayTime}}</time></td>
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
    </div>
    {{if gt .TotalPages 1}}
    <nav class="pagination">
      {{if .FirstPageLink}}<a href="{{.FirstPageLink}}">First</a>{{else}}<span class="disabled">First</span>{{end}}
      {{if .PrevPageLink}}<a href="{{.PrevPageLink}}">← Prev</a>{{else}}<span class="disabled">← Prev</span>{{end}}
      <span class="page-info">Page {{.Page}} of {{.TotalPages}}</span>
      {{if .NextPageLink}}<a href="{{.NextPageLink}}">Next →</a>{{else}}<span class="disabled">Next →</span>{{end}}
      {{if .LastPageLink}}<a href="{{.LastPageLink}}">Last</a>{{else}}<span class="disabled">Last</span>{{end}}
    </nav>
    {{end}}
    {{else}}
    <p class="empty">{{if .Filters.Active}}No alerts match the current filters.{{else}}No alerts yet. Enable Alertmanager polling or point Alertmanager at <code>/webhook</code>.{{end}}</p>
    {{end}}
    </div>
  </div>
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
      var fromDate = document.getElementById('from-date');
      var fromTime = document.getElementById('from-time');
      var toDate = document.getElementById('to-date');
      var toTime = document.getElementById('to-time');
      var applyDates = document.getElementById('apply-dates');
      var modeRadios = form.querySelectorAll('input[name="custom_mode"]');
      var time24Pattern = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;

      function isValidTime24(value) {
        return time24Pattern.test((value || '').trim());
      }

      function combineDateTime(dateEl, timeEl, defaultTime) {
        if (!dateEl || !dateEl.value) return '';
        var time = (timeEl && timeEl.value.trim()) || defaultTime;
        if (!isValidTime24(time)) return '';
        return dateEl.value + 'T' + time;
      }

      function syncFromToHidden() {
        if (from) from.value = combineDateTime(fromDate, fromTime, '00:00');
        if (to) to.value = combineDateTime(toDate, toTime, '23:59');
      }

      function pad2(n) {
        return String(n).padStart(2, '0');
      }

      function utcDateValue(d) {
        return d.getUTCFullYear() + '-' + pad2(d.getUTCMonth() + 1) + '-' + pad2(d.getUTCDate());
      }

      function utcTimeValue(d) {
        return pad2(d.getUTCHours()) + ':' + pad2(d.getUTCMinutes());
      }

      function calendarRangeIsEmpty() {
        return !(fromDate && fromDate.value) && !(fromTime && fromTime.value.trim()) &&
          !(toDate && toDate.value) && !(toTime && toTime.value.trim()) &&
          !(from && from.value) && !(to && to.value);
      }

      function setDefaultCalendarRange() {
        if (!calendarRangeIsEmpty()) return;
        var now = new Date();
        var fromMs = new Date(now.getTime() - 30 * 60 * 1000);
        if (fromDate) fromDate.value = utcDateValue(fromMs);
        if (fromTime) fromTime.value = utcTimeValue(fromMs);
        if (toDate) toDate.value = utcDateValue(now);
        if (toTime) toTime.value = utcTimeValue(now);
        syncFromToHidden();
      }

      function setCalendarInputsDisabled(disabled) {
        if (from) from.disabled = disabled;
        if (to) to.disabled = disabled;
        if (fromDate) fromDate.disabled = disabled;
        if (fromTime) fromTime.disabled = disabled;
        if (toDate) toDate.disabled = disabled;
        if (toTime) toTime.disabled = disabled;
      }

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
        if (fromDate) fromDate.value = '';
        if (fromTime) fromTime.value = '';
        if (toDate) toDate.value = '';
        if (toTime) toTime.value = '';
      }

      function prepareCustomSubmit() {
        if (range) range.value = 'custom';
        var mode = customMode();
        if (mode === 'days') {
          setCalendarInputsDisabled(true);
          if (days) days.disabled = false;
        } else {
          if (days) { days.value = ''; days.disabled = true; }
          setCalendarInputsDisabled(false);
          syncFromToHidden();
        }
      }

      function enableCustomInputs() {
        if (days) days.disabled = false;
        setCalendarInputsDisabled(false);
      }

      form.querySelectorAll('select').forEach(function (el) {
        el.addEventListener('change', function () {
          if (el === range) {
            if (el.value === 'custom') {
              showPanel(true);
              syncModePanels();
              enableCustomInputs();
              if (customMode() === 'calendar') {
                setDefaultCalendarRange();
              }
              return;
            }
            showPanel(false);
            clearCustomInputs();
          }
          form.submit();
        });
      });

      modeRadios.forEach(function (radio) {
        radio.addEventListener('change', function () {
          syncModePanels();
          if (radio.value === 'calendar' && radio.checked) {
            setDefaultCalendarRange();
          }
        });
      });

      if (applyDates) {
        applyDates.addEventListener('click', function (e) {
          e.preventDefault();
          prepareCustomSubmit();
          form.submit();
        });
      }

      form.addEventListener('submit', function () {
        if (customMode() === 'calendar') {
          syncFromToHidden();
        }
      });

      if (isCustomRange()) {
        showPanel(true);
        syncModePanels();
        if (customMode() === 'calendar') {
          setDefaultCalendarRange();
        }
      }

      var alertname = document.getElementById('alertname');
      if (alertname) {
        alertname.addEventListener('keydown', function (e) {
          if (e.key === 'Enter') { e.preventDefault(); form.submit(); }
        });
      }
    })();
` + UTCClockRefreshJS + RelativeTimeRefreshJS + `
  </script>
</body>
</html>`))

type Server struct {
	store      *AlertStore
	slackToken string
	channelID  string

	mu          sync.Mutex
	dailyThread map[string]string

	snapshot *AlertSnapshotCache

	clusterMetaMu         sync.RWMutex
	clusterMeta           ClusterMeta
	clusterMetaAt         time.Time
	clusterMetaRefreshing int32
}

// NewServer constructs an HTTP handler bundle for the dashboard and webhook.
func NewServer(store *AlertStore, slackToken, channelID string) *Server {
	s := &Server{
		store:       store,
		slackToken:  slackToken,
		channelID:   channelID,
		dailyThread: map[string]string{},
	}
	if store != nil {
		s.snapshot = newAlertSnapshotCache(store)
		store.SetChangeNotifier(s.snapshot.MarkDirty)
		s.snapshot.start()
		go s.refreshClusterMetaLoop()
	}
	return s
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

	var (
		total int
		view  filteredAlertView
	)
	if s.snapshot != nil {
		if n, err := s.store.Len(ctx); err == nil && int(n) != s.snapshot.Total() {
			s.snapshot.RefreshNow(ctx)
		}
		snapView := s.snapshot.filteredView(filters)
		total = s.snapshot.Total()
		view = snapView
	} else {
		var err error
		allFromStore, err := s.store.List(ctx, 0)
		if err != nil {
			log.Printf("list alerts: %v", err)
			http.Error(w, "failed to load alerts", http.StatusInternalServerError)
			return
		}
		total = len(allFromStore)
		filtered := FilterAlerts(allFromStore, filters)
		view = filteredAlertView{
			alerts:            filtered,
			namespaceRanks:    RankAlertsByNamespace(filtered),
			namespaces:        NamespacesForFilter(filtered, filters.Namespace),
			hasEmptyNamespace: AlertsHaveEmptyNamespace(filtered),
		}
	}

	alerts := view.alerts
	pageAlerts, totalPages, page := PaginateAlerts(alerts, filters.Page, filters.PerPage)
	filters.Page = page

	pageStart, pageEnd := 0, 0
	if len(pageAlerts) > 0 {
		pageStart = (page-1)*filters.PerPage + 1
		pageEnd = pageStart + len(pageAlerts) - 1
	}

	var buf bytes.Buffer
	if err := alertsTemplate.Execute(&buf, AlertsPageData{
		Alerts:                  pageAlerts,
		Count:                   len(pageAlerts),
		Filtered:                len(alerts),
		Total:                   total,
		Filters:                 filters,
		Namespaces:              view.namespaces,
		NamespaceRanks:          view.namespaceRanks,
		HasEmptyNamespaceAlerts: view.hasEmptyNamespace,
		Page:                    page,
		TotalPages:              totalPages,
		PageStart:               pageStart,
		PageEnd:                 pageEnd,
		Cluster:                 s.cachedClusterMeta(),
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
	if len(payload.Alerts) > 0 && s.snapshot != nil {
		s.snapshot.MarkDirty()
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
