#!/usr/bin/env bash
# Apply KContext OpenShift RBAC and mint a bearer token for local Alertmanager polling.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

NAMESPACE="${KCONTEXT_NAMESPACE:-kcontext}"
SA="${KCONTEXT_SA:-kcontext}"
DURATION="${KCONTEXT_TOKEN_DURATION:-8760h}"
TOKEN_FILE="${KCONTEXT_TOKEN_FILE:-}"
WRITE_FILE=false

usage() {
	cat <<EOF
Usage: $(basename "$0") [options]

Apply deploy/openshift/ RBAC and print exports for ALERTMANAGER_TOKEN.

Options:
  -n, --namespace NAME   ServiceAccount namespace (default: kcontext)
  -s, --serviceaccount   ServiceAccount name (default: kcontext)
  -d, --duration DUR     Token lifetime for oc create token (default: 8760h / 1 year)
  -f, --token-file PATH  Write token to PATH and export ALERTMANAGER_TOKEN_FILE
  -h, --help             Show this help

Environment:
  KCONTEXT_NAMESPACE, KCONTEXT_SA, KCONTEXT_TOKEN_DURATION, KCONTEXT_TOKEN_FILE

Example:
  eval "\$($(basename "$0"))"
  go run ./cmd/kcontext
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	-n | --namespace)
		NAMESPACE="$2"
		shift 2
		;;
	-s | --serviceaccount)
		SA="$2"
		shift 2
		;;
	-d | --duration)
		DURATION="$2"
		shift 2
		;;
	-f | --token-file)
		TOKEN_FILE="$2"
		WRITE_FILE=true
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if ! command -v oc >/dev/null 2>&1; then
	echo "oc not found in PATH (run oc login first)" >&2
	exit 1
fi

echo "Applying RBAC in ${NAMESPACE}..." >&2
oc apply -f "${REPO_ROOT}/deploy/openshift/" >&2

echo "Creating token for serviceaccount/${SA} in ${NAMESPACE} (duration=${DURATION})..." >&2
TOKEN="$(oc create token "${SA}" -n "${NAMESPACE}" --duration="${DURATION}")"

if [[ -z "${TOKEN}" ]]; then
	echo "oc create token returned an empty token" >&2
	exit 1
fi

if [[ "${WRITE_FILE}" == true ]]; then
	mkdir -p "$(dirname "${TOKEN_FILE}")"
	printf '%s' "${TOKEN}" >"${TOKEN_FILE}"
	chmod 600 "${TOKEN_FILE}"
	echo "export ALERTMANAGER_TOKEN_FILE=${TOKEN_FILE}"
else
	echo "export ALERTMANAGER_TOKEN=${TOKEN}"
fi

echo "# Token expires after ${DURATION}; re-run this script to refresh." >&2
echo "# Verify (with port-forward to alertmanager-main on 9094):" >&2
echo "# curl -sk -H \"Authorization: Bearer \${ALERTMANAGER_TOKEN:-\$(cat \${ALERTMANAGER_TOKEN_FILE})}\" https://localhost:9094/api/v2/alerts | head" >&2
