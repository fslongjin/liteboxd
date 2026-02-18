#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
KUBECTL_BIN="${KUBECTL_BIN:-kubectl}"
APPLY_SANDBOX="${APPLY_SANDBOX:-true}"
REGISTRY="${REGISTRY:-}"
TAG="${TAG:-latest}"

usage() {
  echo "Usage: REGISTRY=<registry> [TAG=<tag>] $0"
  echo ""
  echo "  REGISTRY  镜像仓库地址，例如: <your registry>/liteboxd"
  echo "  TAG       镜像标签，默认: latest"
  echo ""
  echo "Optional:"
  echo "  KUBECTL_BIN=kubectl APPLY_SANDBOX=true|false"
  echo ""
  echo "Example:"
  echo "  REGISTRY=<your registry>/liteboxd TAG=test1 $0"
  exit 1
}

if [[ -z "${REGISTRY}" ]]; then
  usage
fi

# Remove trailing "/" if present
REGISTRY="${REGISTRY%/}"
API_IMAGE="${REGISTRY}/liteboxd-server:${TAG}"
GATEWAY_IMAGE="${REGISTRY}/liteboxd-gateway:${TAG}"
WEB_IMAGE="${REGISTRY}/web:${TAG}"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

if [[ ! -d "${DEPLOY_DIR}/system" ]]; then
  echo "Error: system manifests not found at ${DEPLOY_DIR}/system" >&2
  exit 1
fi

# Use a temp workspace with relative paths; kubectl -k may reject absolute resource paths.
cp -R "${DEPLOY_DIR}/system" "${TMP_DIR}/system"

cat > "${TMP_DIR}/kustomization.yaml" <<KUSTOM
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ./system
patches:
  - target:
      kind: Deployment
      name: liteboxd-api
    patch: |-
      - op: replace
        path: /spec/template/spec/containers/0/image
        value: ${API_IMAGE}
  - target:
      kind: Deployment
      name: liteboxd-gateway
    patch: |-
      - op: replace
        path: /spec/template/spec/containers/0/image
        value: ${GATEWAY_IMAGE}
  - target:
      kind: Deployment
      name: liteboxd-web
    patch: |-
      - op: replace
        path: /spec/template/spec/containers/0/image
        value: ${WEB_IMAGE}
KUSTOM

echo "[Deploy] Applying control-plane manifests with runtime image overrides..."
"${KUBECTL_BIN}" apply -k "${TMP_DIR}"

if [[ "${APPLY_SANDBOX}" == "true" ]]; then
  if [[ ! -d "${DEPLOY_DIR}/sandbox" ]]; then
    echo "Error: sandbox manifests not found at ${DEPLOY_DIR}/sandbox" >&2
    exit 1
  fi
  echo "[Deploy] Applying sandbox manifests..."
  "${KUBECTL_BIN}" apply -k "${DEPLOY_DIR}/sandbox"
fi

echo "[Deploy] Done."
