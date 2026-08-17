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
- For Alertmanager polling with `oc port-forward`: `oc` logged in on the install user; `install-kcontext.sh` configures `User` and `KUBECONFIG` automatically when `~/.kube/config` exists

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
8. Validates prerequisites (`oc whoami`, `oc apply` RBAC when polling needs `oc`; token file when set)
9. Runs `systemctl daemon-reload`, `enable`, and `restart` (or `start` if not running)
10. Waits until the service stays active for 8+ seconds, then prints `systemctl status`

Install **fails with an error** (not just a warning) if kubeconfig is missing, `oc` is not logged in, RBAC apply fails, or the service crashes on startup.

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
| `/etc/kcontext/kubeconfig` | Flattened kubeconfig for systemd (from installer `$KUBECONFIG` at install time) |
| `/etc/systemd/system/kcontext.service` | systemd unit |
| `/etc/systemd/system/kcontext.service.d/install.conf` | Auto-generated `User` / `KUBECONFIG` for `oc` |

## Local dev with `oc` port-forward

When you run `install-kcontext.sh`, the script detects a working `oc` login using:

1. `KCONTEXT_INSTALL_KUBECONFIG` (if set)
2. **`$KUBECONFIG` from your shell** (full value, including colon-separated paths)
3. `oc whoami` in the current shell
4. Login shell / `~/.kube/config` fallback

It materializes credentials to `/etc/kcontext/kubeconfig` and writes `/etc/systemd/system/kcontext.service.d/install.conf` with:

- `User` / `Group` — the install user (`SUDO_USER`, `KCONTEXT_INSTALL_USER`, or `root`)
- `KUBECONFIG` — `/etc/kcontext/kubeconfig`

Override manually:

```bash
export KUBECONFIG=/path/to/your/kubeconfig   # or export before install
oc login ...
./utils/install-kcontext.sh
```

Or:

```bash
export KCONTEXT_INSTALL_USER=youruser
export KCONTEXT_INSTALL_KUBECONFIG=/home/youruser/.kube/config
./utils/install-kcontext.sh
```

If `oc` is not configured, set `ALERTMANAGER_TOKEN_FILE` in `/etc/kcontext/kcontext.env` or disable polling with `ALERTMANAGER_URL=`.

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
