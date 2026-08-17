#!/usr/bin/env bash
# Build KContext, install binary + systemd unit, and restart the service.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BIN_DEST="/usr/local/bin/kcontext"
ENV_DIR="/etc/kcontext"
ENV_FILE="${ENV_DIR}/kcontext.env"
SYSTEMD_UNIT="/etc/systemd/system/kcontext.service"
SERVICE_NAME="kcontext"
BUILD_OUTPUT="${SCRIPT_DIR}/.kcontext.build"

usage() {
	cat <<EOF
Usage: $(basename "$0") [options]

Build KContext from this repo, install to ${BIN_DEST}, refresh the systemd unit,
and enable/restart the ${SERVICE_NAME} service.

Options:
  --no-start       Install only; do not enable or restart the service
  --no-test        Skip go test before build
  -h, --help       Show this help

First run creates ${ENV_FILE} from kcontext.env.example if missing.
An existing env file is never overwritten.
EOF
}

run_root() {
	if [[ "${EUID}" -eq 0 ]]; then
		"$@"
	else
		sudo "$@"
	fi
}

NO_START=false
NO_TEST=false

while [[ $# -gt 0 ]]; do
	case "$1" in
	--no-start)
		NO_START=true
		shift
		;;
	--no-test)
		NO_TEST=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Unknown option: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

cd "${REPO_ROOT}"

if ! command -v go >/dev/null 2>&1; then
	echo "go is required but not found in PATH" >&2
	exit 1
fi

echo "==> Building ${SERVICE_NAME}..."
if [[ "${NO_TEST}" == false ]]; then
	go test ./...
fi
go build -o "${BUILD_OUTPUT}" ./cmd/kcontext

echo "==> Installing binary to ${BIN_DEST}..."
run_root install -m 0755 "${BUILD_OUTPUT}" "${BIN_DEST}"
rm -f "${BUILD_OUTPUT}"

echo "==> Installing config and systemd unit..."
run_root install -d -m 0750 "${ENV_DIR}"
if [[ -f "${ENV_FILE}" ]]; then
	echo "    Keeping existing ${ENV_FILE}"
else
	run_root install -m 0640 "${SCRIPT_DIR}/kcontext.env.example" "${ENV_FILE}"
	echo "    Created ${ENV_FILE} from example — edit before production use"
fi
run_root install -m 0644 "${SCRIPT_DIR}/kcontext.service" "${SYSTEMD_UNIT}"

echo "==> Reloading systemd..."
run_root systemctl daemon-reload

if [[ "${NO_START}" == true ]]; then
	echo "==> Skipping enable/start (--no-start)"
	exit 0
fi

echo "==> Enabling and restarting ${SERVICE_NAME}..."
run_root systemctl enable "${SERVICE_NAME}"
if run_root systemctl is-active --quiet "${SERVICE_NAME}"; then
	run_root systemctl restart "${SERVICE_NAME}"
else
	run_root systemctl start "${SERVICE_NAME}"
fi

echo "==> Status:"
run_root systemctl --no-pager status "${SERVICE_NAME}"
