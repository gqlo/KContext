# Dashboard

`GET /` loads all alerts from Redis, applies filters, paginates, and renders server-side HTML via Go's `html/template` (embedded in `handlers.go`).

Default URL: [http://localhost:8083/](http://localhost:8083/) (override host/port with `LISTEN_ADDR`).

## Display

- Table columns: updated at, status, alert name, namespace, severity, source, summary, runbook link
- Rows sorted by **Updated at** (newest first), then by alert ID
- Row background color reflects severity for firing alerts (critical / warning / info)
- Namespace values are clickable — filter to that namespace
- Runbook links come from `annotations.runbook_url` (or `runbook`)
- **KContext** title links to `/` and clears all filters
- 200 alerts per page

## Filters

Filters auto-apply on dropdown change (alert name requires Enter).

| Query param | Values |
|-------------|--------|
| `severity` | `critical`, `warning`, `info` |
| `status` | `firing`, `resolved`, `suppressed` |
| `source` | `poll`, `webhook` |
| `range` | `today`, `7d`, `14d`, `30d`, `custom` |
| `days` | with `range=custom` — past N days rolling window |
| `from` / `to` | with `range=custom` — calendar range (`YYYY-MM-DD`, inclusive) |
| `namespace` | exact namespace name |
| `alertname` | substring match |
| `page` | page number |

### Examples

```text
/?range=today&severity=critical&namespace=openshift-monitoring
/?range=today&severity=critical&namespace=openshift-monitoring&page=2
/?range=custom&days=3
/?range=custom&from=2026-06-28&to=2026-06-29
```

### Custom date range

Choose **Custom…** in the Date dropdown:

- **Past N days** — enter a number and click Apply
- **Calendar range** — pick From/To dates and click Apply

## Alert detail

Click an alert row or open directly:

```text
GET /alert?id=<stored-alert-id>
```

See [endpoints.md](endpoints.md) for all HTTP routes.
