#!/usr/bin/env bash
# Build KContext, install binary + systemd unit, and restart the service.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BIN_DEST="/usr/local/bin/kcontext"
DEPLOY_DEST="/opt/kcontext/deploy/openshift"
ENV_DIR="/etc/kcontext"
ENV_FILE="${ENV_DIR}/kcontext.env"
SYSTEMD_UNIT="/etc/systemd/system/kcontext.service"
SYSTEMD_DROPIN_DIR="/etc/systemd/system/kcontext.service.d"
SYSTEMD_DROPIN="${SYSTEMD_DROPIN_DIR}/install.conf"
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
An existing env file is never overwritten (except KCONTEXT_DEPLOY_DIR and KUBECONFIG are refreshed when detected).

When Alertmanager polling is enabled, install validates oc login, oc apply RBAC manifests,
and that the service stays running for at least 8 seconds after start.
EOF
}

install_user() {
	if [[ -n "${KCONTEXT_INSTALL_USER:-}" ]]; then
		echo "${KCONTEXT_INSTALL_USER}"
		return
	fi
	if [[ -n "${SUDO_USER:-}" ]]; then
		echo "${SUDO_USER}"
		return
	fi
	if [[ "${EUID}" -ne 0 ]]; then
		echo "${USER}"
		return
	fi
	# Root shell (no SUDO_USER): use root's home and kubeconfig if present.
	echo "root"
}

# Read a key from the installed env file (last match wins).
read_env_var() {
	local key="$1"
	if ! run_root test -f "${ENV_FILE}"; then
		return 1
	fi
	local line value
	line="$(run_root grep -E "^${key}=" "${ENV_FILE}" 2>/dev/null | tail -1 || true)"
	if [[ -z "${line}" ]]; then
		return 1
	fi
	value="${line#*=}"
	value="${value%\"}"
	value="${value#\"}"
	printf '%s' "${value}"
}

# Return 0 when Alertmanager polling needs oc/kubeconfig (no pre-created token file).
alertmanager_needs_oc() {
	local url token_file
	url="$(read_env_var ALERTMANAGER_URL || true)"
	if [[ -n "${url}" && -z "${url// }" ]]; then
		return 1
	fi
	token_file="$(read_env_var ALERTMANAGER_TOKEN_FILE || true)"
	if [[ -n "${token_file}" ]] && run_root test -f "${token_file}"; then
		return 1
	fi
	return 0
}

validate_alertmanager_token_file() {
	local token_file token
	token_file="$(read_env_var ALERTMANAGER_TOKEN_FILE || true)"
	if [[ -z "${token_file}" ]]; then
		return 0
	fi
	if ! run_root test -f "${token_file}"; then
		echo "ERROR: ALERTMANAGER_TOKEN_FILE=${token_file} does not exist" >&2
		return 1
	fi
	token="$(run_root cat "${token_file}" 2>/dev/null | tr -d '[:space:]' || true)"
	if [[ -z "${token}" ]]; then
		echo "ERROR: ALERTMANAGER_TOKEN_FILE=${token_file} is empty" >&2
		return 1
	fi
	echo "    ALERTMANAGER_TOKEN_FILE ok (${token_file})"
	return 0
}

validate_oc_kubeconfig() {
	local user home kubeconfig whoami
	user="$(install_user)"
	if [[ -z "${user}" ]]; then
		echo "ERROR: could not determine install user for oc/kubeconfig" >&2
		echo "    Run via sudo from your login, set KCONTEXT_INSTALL_USER, or edit ${SYSTEMD_DROPIN}" >&2
		return 1
	fi

	home="$(getent passwd "${user}" | cut -d: -f6)"
	if [[ -z "${home}" ]]; then
		echo "ERROR: no passwd entry for user ${user}" >&2
		return 1
	fi

	kubeconfig="${KCONTEXT_INSTALL_KUBECONFIG:-${home}/.kube/config}"
	if ! run_root test -f "${kubeconfig}"; then
		echo "ERROR: kubeconfig not found at ${kubeconfig} for user ${user}" >&2
		echo "    Run oc login, set KCONTEXT_INSTALL_KUBECONFIG, or set ALERTMANAGER_TOKEN_FILE" >&2
		return 1
	fi

	if ! command -v oc >/dev/null 2>&1; then
		echo "ERROR: oc not found in PATH (required for Alertmanager polling)" >&2
		return 1
	fi

	whoami="$(KUBECONFIG="${kubeconfig}" oc whoami 2>&1)" || {
		echo "ERROR: kubeconfig at ${kubeconfig} is missing or incomplete (oc whoami failed)" >&2
		echo "    ${whoami}" >&2
		echo "    Run oc login or fix KUBECONFIG before installing" >&2
		return 1
	}
	echo "    oc whoami: ${whoami} (KUBECONFIG=${kubeconfig})"

	if ! KUBECONFIG="${kubeconfig}" oc apply -f "${DEPLOY_DEST}" >/dev/null 2>&1; then
		echo "ERROR: oc apply -f ${DEPLOY_DEST} failed (same step kcontext runs at startup)" >&2
		KUBECONFIG="${kubeconfig}" oc apply -f "${DEPLOY_DEST}" >&2 || true
		return 1
	fi
	echo "    oc apply -f ${DEPLOY_DEST} ok"

	# Stash for install_service_identity
	KCONTEXT_VALIDATED_KUBECONFIG="${kubeconfig}"
	KCONTEXT_VALIDATED_USER="${user}"
	return 0
}

validate_prerequisites() {
	echo "==> Validating prerequisites..."
	if alertmanager_needs_oc; then
		validate_oc_kubeconfig || return 1
	else
		echo "    Alertmanager polling disabled or using ALERTMANAGER_TOKEN_FILE"
		validate_alertmanager_token_file || return 1
	fi
	return 0
}

install_service_identity() {
	local user home group kubeconfig
	user="${KCONTEXT_VALIDATED_USER:-$(install_user)}"
	kubeconfig="${KCONTEXT_VALIDATED_KUBECONFIG:-}"

	if [[ -z "${user}" || -z "${kubeconfig}" ]]; then
		echo "ERROR: service identity not validated — internal error" >&2
		run_root rm -f "${SYSTEMD_DROPIN}"
		return 1
	fi

	home="$(getent passwd "${user}" | cut -d: -f6)"
	if [[ -z "${home}" ]]; then
		echo "ERROR: no passwd entry for user ${user}" >&2
		run_root rm -f "${SYSTEMD_DROPIN}"
		return 1
	fi

	group="$(id -gn "${user}")"

	echo "==> Configuring service identity (User=${user}, KUBECONFIG=${kubeconfig})..."
	run_root install -d -m 0755 "${SYSTEMD_DROPIN_DIR}"
	run_root tee "${SYSTEMD_DROPIN}" >/dev/null <<EOF
# Generated by install-kcontext.sh — re-run install to refresh
[Service]
User=${user}
Group=${group}
Environment=KUBECONFIG=${kubeconfig}
EOF
	run_root chmod 0644 "${SYSTEMD_DROPIN}"
	set_env_var "${ENV_FILE}" KUBECONFIG "${kubeconfig}"
	return 0
}

verify_service_started() {
	local elapsed=0 stable=0 state
	# kcontext may spend ~5s on oc apply before exiting on auth errors.
	while (( elapsed < 20 )); do
		state="$(run_root systemctl is-active "${SERVICE_NAME}" 2>/dev/null || true)"
		case "${state}" in
		active)
			(( stable += 2 ))
			if (( stable >= 8 )); then
				return 0
			fi
			;;
		failed | inactive | dead)
			break
			;;
		esac
		sleep 2
		(( elapsed += 2 ))
	done
	echo "ERROR: ${SERVICE_NAME} failed to stay running after install." >&2
	echo "    Check: journalctl -u ${SERVICE_NAME} -n 30 --no-pager" >&2
	run_root journalctl -u "${SERVICE_NAME}" -n 15 --no-pager >&2 || true
	run_root systemctl --no-pager status "${SERVICE_NAME}" >&2 || true
	return 1
}

set_env_var() {
	local file="$1"
	local key="$2"
	local value="$3"
	if ! run_root test -f "${file}"; then
		return
	fi
	if run_root grep -q "^${key}=" "${file}"; then
		run_root sed -i "s|^${key}=.*|${key}=${value}|" "${file}"
	else
		printf '%s=%s\n' "${key}" "${value}" | run_root tee -a "${file}" >/dev/null
	fi
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

echo "==> Installing OpenShift RBAC manifests to ${DEPLOY_DEST}..."
if [[ ! -d "${REPO_ROOT}/deploy/openshift" ]]; then
	echo "deploy/openshift not found in ${REPO_ROOT}" >&2
	exit 1
fi
run_root install -d -m 0755 "${DEPLOY_DEST}"
run_root cp -a "${REPO_ROOT}/deploy/openshift/." "${DEPLOY_DEST}/"

echo "==> Installing config and systemd unit..."
run_root install -d -m 0750 "${ENV_DIR}"
if [[ -f "${ENV_FILE}" ]]; then
	echo "    Keeping existing ${ENV_FILE}"
else
	run_root install -m 0640 "${SCRIPT_DIR}/kcontext.env.example" "${ENV_FILE}"
	echo "    Created ${ENV_FILE} from example — edit before production use"
fi
set_env_var "${ENV_FILE}" KCONTEXT_DEPLOY_DIR "${DEPLOY_DEST}"
echo "    Set KCONTEXT_DEPLOY_DIR=${DEPLOY_DEST}"
run_root install -m 0644 "${SCRIPT_DIR}/kcontext.service" "${SYSTEMD_UNIT}"

validate_prerequisites || exit 1

if alertmanager_needs_oc; then
	install_service_identity || exit 1
else
	echo "==> Skipping oc/kubeconfig drop-in (Alertmanager polling disabled or token file set)"
	run_root rm -f "${SYSTEMD_DROPIN}"
fi

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

verify_service_started || exit 1

echo "==> Status:"
run_root systemctl --no-pager status "${SERVICE_NAME}"
