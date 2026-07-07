# Webhook ingest

`POST /webhook` accepts [Prometheus Alertmanager webhook JSON](https://prometheus.io/docs/alerting/latest/configuration/#webhook_config). Each alert in the payload is saved immediately with `source: webhook` and appears on the dashboard.

See [alert-flow.md](alert-flow.md) for how webhook ingest fits alongside polling.

## Alertmanager receiver config

```yaml
receivers:
  - name: kcontext
    webhook_configs:
      - url: http://kcontext:8083/webhook
        send_resolved: true
```

Use the in-cluster service URL when KContext runs on the cluster, or a reachable host/port for local dev.

## Webhook-only mode

If you only use webhooks (no polling), disable the poller:

```bash
export ALERTMANAGER_URL=""
go run ./cmd/kcontext
```

Prerequisites: Go and Redis only — no `oc` or Alertmanager API access required.

## Test locally

```bash
./docs/test-webhook.sh
```

Payload: [webhook-payload.json](webhook-payload.json). Override the target with `KCONTEXT_URL` (default `http://localhost:8083`).

Refresh the dashboard at [http://localhost:8083/](http://localhost:8083/) to see the alert.

## Related

| Topic | Example |
|-------|---------|
| HTTP routes | [endpoints.md](endpoints.md) |
| Ingest diagram | [alert-flow.md](alert-flow.md) |
| Environment variables | [configuration.md](configuration.md) |
