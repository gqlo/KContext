# KContext

**Repository:** [github.com/gqlo/KContext](https://github.com/gqlo/KContext)

Ingest OpenShift / Kubernetes Alertmanager alerts, store them in Redis, and browse them on a filterable HTML dashboard — with optional Slack notifications and cluster context via `oc`.

When something fires, operators need more than the alert text. KContext gathers surrounding signals into a structured bundle so you can debug faster or hand off a concise summary to an LLM.

## What it does today

- **Poll** Alertmanager (`GET /api/v2/alerts`) on an interval
- **Receive** Alertmanager webhooks at `POST /webhook`
- **Store** alerts in Redis with deduplication and resolved detection
- **Browse** a filterable HTML dashboard at `/`

**Planned:** log extraction via `oc`, event scraping, context bundling, AI analysis.

## Prerequisites

| Requirement | Required for | Notes |
|-------------|--------------|-------|
| **Go 1.24+** | Building and running | See `go.mod` |
| **Redis** | Always | Stores alerts; default `localhost:6379` |
| **`oc` CLI** | Alert polling (local dev) | Logged in to an OpenShift cluster |
| **Alertmanager** | Poll and/or webhook | Poll needs API access; webhook-only mode needs no poll setup |

Start Redis locally if you do not already have it:

```bash
# Fedora / RHEL
sudo dnf install redis && sudo systemctl start redis
```

For **webhook-only** use (no polling), you only need Go and Redis — point Alertmanager at `POST /webhook` and set `ALERTMANAGER_URL=""` to disable polling.

## Quick start

```bash
export REDIS_ADDR=localhost:6379
go run ./cmd/kcontext
```

Open [http://localhost:8083/](http://localhost:8083/).

Polling defaults to `https://localhost:9094` and starts `oc port-forward` automatically. See [docs/alert-polling.md](docs/alert-polling.md) for API details, OpenShift RBAC, and manual testing.

## Documentation

| Doc | Description |
|-----|-------------|
| [architecture.md](docs/architecture.md) | How KContext works today — pieces and data flow |
| [alert-flow.md](docs/alert-flow.md) | Poll vs webhook ingest paths |
| [webhook-ingest.md](docs/webhook-ingest.md) | Alertmanager webhook receiver and testing |
| [configuration.md](docs/configuration.md) | Environment variables |
| [endpoints.md](docs/endpoints.md) | HTTP routes |
| [dashboard.md](docs/dashboard.md) | Dashboard UI, filters, and pagination |
| [alert-polling.md](docs/alert-polling.md) | Alertmanager poll API, OpenShift RBAC, dedup |
| [vm-context.md](docs/vm-context.md) | Design: VM alert context (describe VMI + virt-launcher logs) |
| [alertmanager-api-response.json](docs/alertmanager-api-response.json) | Sample Alertmanager poll response |
| [test-webhook.sh](docs/test-webhook.sh) | POST a sample alert to `/webhook` |
| [webhook-payload.json](docs/webhook-payload.json) | JSON body used by `test-webhook.sh` |
| [redis-migration.md](docs/redis-migration.md) | Copy alert history between Redis instances |
