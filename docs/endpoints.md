# HTTP endpoints

Default listen address: `:8083` (override with `LISTEN_ADDR`).

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | HTML dashboard — see [dashboard.md](dashboard.md) |
| `GET` | `/alert?id={id}` | Alert detail page for a stored alert ID |
| `POST` | `/webhook` | Alertmanager webhook receiver — see [webhook-ingest.md](webhook-ingest.md) |

## Alert detail

Open a single alert by ID (shown in the dashboard). Filter and display options: [dashboard.md](dashboard.md).

```text
GET /alert?id=<stored-alert-id>
```

Planned: for VM-related alerts, the detail page also shows collected VMI describe and virt-launcher logs — see [vm-context.md](vm-context.md).
