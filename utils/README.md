# KContext systemd install

Run KContext as a systemd service on Linux. Files in this directory:

| File | Purpose |
|------|---------|
| `install-kcontext.sh` | Build, install, enable, and restart (one command for updates) |
| `kcontext.service` | systemd unit installed to `/etc/systemd/system/` |
| `kcontext.env.example` | Template for `/etc/kcontext/kcontext.env` |

## Prerequisites

- Go 1.24+ (to build)
- Redis running (`redis.service` or remote `REDIS_ADDR`)
- For Alertmanager polling with `oc port-forward`: `oc` logged in; configure `User` and `KUBECONFIG` in the unit file (see below)

## Quick install

From the repository root:

```bash
./utils/install-kcontext.sh
```

That script:

1. Runs `go test ./...` (unless `--no-test`)
2. Builds `./cmd/kcontext` into a temporary binary
3. Installs the binary to `/usr/local/bin/kcontext` (replaces an existing install)
4. Copies `deploy/openshift/` to `/opt/kcontext/deploy/openshift`
5. Creates `/etc/kcontext/kcontext.env` from `kcontext.env.example` **only if it does not exist**
6. Sets `KCONTEXT_DEPLOY_DIR=/opt/kcontext/deploy/openshift` in the env file (every install)
7. Installs `kcontext.service` to `/etc/systemd/system/`
8. Runs `systemctl daemon-reload`, `enable`, and `restart` (or `start` if not running)
9. Prints `systemctl status`

Open [http://localhost:8083/](http://localhost:8083/) (or your `LISTEN_ADDR`).

## Updating after code changes

Run the same script again:

```bash
./utils/install-kcontext.sh
```

Each run **rebuilds** from the current repo and **overwrites** `/usr/local/bin/kcontext` via `install` (no separate uninstall). OpenShift RBAC manifests are refreshed under `/opt/kcontext/deploy/openshift`. The service is restarted so the new binary is loaded.

Other `kcontext.env` settings are **not** overwritten on updates (only `KCONTEXT_DEPLOY_DIR` is refreshed).

## Script options

| Option | Effect |
|--------|--------|
| `--no-test` | Skip `go test ./...` before build |
| `--no-start` | Install binary and unit only; do not enable or restart the service |
| `-h`, `--help` | Show usage |

Examples:

```bash
./utils/install-kcontext.sh --no-test          # faster iteration while developing
./utils/install-kcontext.sh --no-start       # install only, start manually later
```

## Configuration

Edit `/etc/kcontext/kcontext.env` (see `kcontext.env.example` for all variables). See [configuration.md](../docs/configuration.md) for details.

After changing the env file:

```bash
sudo systemctl restart kcontext
```

## systemd paths

| Path | Description |
|------|-------------|
| `/usr/local/bin/kcontext` | Installed binary |
| `/opt/kcontext/deploy/openshift` | OpenShift RBAC manifests (`oc apply -f`) |
| `/etc/kcontext/kcontext.env` | Environment variables (`EnvironmentFile` in unit) |
| `/etc/systemd/system/kcontext.service` | systemd unit |

## Local dev with `oc` port-forward

The default unit runs as root. Alertmanager auto port-forward needs a valid `kubeconfig`. Edit `/etc/systemd/system/kcontext.service` (or add a drop-in):

```ini
[Service]
User=youruser
Group=youruser
Environment=KUBECONFIG=/home/youruser/.kube/config
```

Then:

```bash
sudo systemctl daemon-reload
sudo systemctl restart kcontext
```

Re-run `install-kcontext.sh` after editing the unit in `utils/kcontext.service` so systemd picks up template changes.

## Manual service commands

```bash
sudo systemctl status kcontext
sudo systemctl restart kcontext
sudo systemctl stop kcontext
journalctl -u kcontext -f
```

## Manual install (without script)

```bash
go build -o kcontext ./cmd/kcontext
sudo install -m 0755 kcontext /usr/local/bin/kcontext
sudo install -d -m 0750 /etc/kcontext
sudo install -m 0640 utils/kcontext.env.example /etc/kcontext/kcontext.env
sudo install -m 0644 utils/kcontext.service /etc/systemd/system/kcontext.service
sudo systemctl daemon-reload
sudo systemctl enable --now kcontext
```
