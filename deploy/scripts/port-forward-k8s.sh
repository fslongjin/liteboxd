#!/usr/bin/env bash
set -euo pipefail

KUBECTL_BIN="${KUBECTL_BIN:-kubectl}"
NS="${CONTROL_NAMESPACE:-liteboxd-system}"
API_SERVICE="${API_SERVICE:-liteboxd-api}"
GATEWAY_SERVICE="${GATEWAY_SERVICE:-liteboxd-gateway}"
API_LOCAL_PORT="${API_LOCAL_PORT:-8080}"
GATEWAY_LOCAL_PORT="${GATEWAY_LOCAL_PORT:-8081}"

PIDS=()

cleanup() {
  for pid in "${PIDS[@]:-}"; do
    if kill -0 "${pid}" >/dev/null 2>&1; then
      kill "${pid}" >/dev/null 2>&1 || true
    fi
  done
}
trap cleanup EXIT INT TERM

echo "[PortForward] namespace=${NS}"
echo "[PortForward] API:     http://127.0.0.1:${API_LOCAL_PORT} -> svc/${API_SERVICE}:8080"
echo "[PortForward] Gateway: http://127.0.0.1:${GATEWAY_LOCAL_PORT} -> svc/${GATEWAY_SERVICE}:8081"

"${KUBECTL_BIN}" -n "${NS}" port-forward "svc/${API_SERVICE}" "${API_LOCAL_PORT}:8080" >/tmp/liteboxd-port-forward-api.log 2>&1 &
PIDS+=("$!")

"${KUBECTL_BIN}" -n "${NS}" port-forward "svc/${GATEWAY_SERVICE}" "${GATEWAY_LOCAL_PORT}:8081" >/tmp/liteboxd-port-forward-gateway.log 2>&1 &
PIDS+=("$!")

sleep 1
for pid in "${PIDS[@]}"; do
  if ! kill -0 "${pid}" >/dev/null 2>&1; then
    echo "[PortForward] Failed to start. Check logs:"
    echo "  /tmp/liteboxd-port-forward-api.log"
    echo "  /tmp/liteboxd-port-forward-gateway.log"
    exit 1
  fi
done

echo "[PortForward] Started. Press Ctrl+C to stop."
wait
