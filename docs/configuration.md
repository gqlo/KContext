# Configuration

All configuration is via environment variables.

## Core

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `REDIS_ADDR` | no | `localhost:6379` | Redis address |
| `LISTEN_ADDR` | no | `:8083` | HTTP listen address |

## Alertmanager polling

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ALERTMANAGER_URL` | no | `https://localhost:9094` | Alertmanager base URL; set to `""` to disable polling |
| `ALERTMANAGER_PORT_FORWARD` | no | `auto` | Start `oc port-forward` when URL is localhost; `false` to disable, `true` to force |
| `ALERTMANAGER_PF_NAMESPACE` | no | `openshift-monitoring` | Namespace for auto port-forward |
| `ALERTMANAGER_PF_SERVICE` | no | `alertmanager-main` | Service name for auto port-forward |
| `ALERTMANAGER_PF_LOCAL_PORT` | no | `9094` | Local port for auto port-forward |
| `ALERTMANAGER_PF_REMOTE_PORT` | no | `9094` | Remote port for auto port-forward |
| `ALERTMANAGER_POLL_INTERVAL` | no | `10s` | How often to poll Alertmanager |
| `ALERTMANAGER_TOKEN_FILE` | no | — | Path to bearer token file; when set, skips auto `oc create token` (in-cluster SA) |
| `ALERTMANAGER_TLS_INSECURE` | no | `true` | Skip TLS verification (`false` for proper TLS) |

## OpenShift token (local dev)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `KCONTEXT_NAMESPACE` | no | `kcontext` | Namespace for auto token ServiceAccount |
| `KCONTEXT_SA` | no | `kcontext` | ServiceAccount name for auto token |
| `KCONTEXT_TOKEN_DURATION` | no | `8760h` | Token lifetime passed to `oc create token` |
| `KCONTEXT_DEPLOY_DIR` | no | `deploy/openshift` | Path to OpenShift RBAC manifests for `oc apply` |

## Example: local dev

```bash
export REDIS_ADDR=localhost:6379
export LISTEN_ADDR=:8083
go run ./cmd/kcontext
```

## Example: in-cluster

```bash
export REDIS_ADDR=redis:6379
export ALERTMANAGER_URL=https://alertmanager-main.openshift-monitoring.svc:9094
export ALERTMANAGER_TOKEN_FILE=/var/run/secrets/kubernetes.io/serviceaccount/token
export ALERTMANAGER_TLS_INSECURE=true
export ALERTMANAGER_PORT_FORWARD=false
```

## Example: webhook test helper

`docs/test-webhook.sh` accepts `KCONTEXT_URL` (default `http://localhost:8083`).
