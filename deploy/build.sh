#!/usr/bin/env bash
set -euo pipefail

# 从 deploy 目录执行时，backend 在上一级
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

REGISTRY="${REGISTRY:-}"
TAG="${TAG:-latest}"
PUSH="${PUSH:-false}"

usage() {
  echo "Usage: REGISTRY=<registry> [TAG=<tag>] [PUSH=true] $0"
  echo ""
  echo "  REGISTRY  镜像仓库地址，例如: myregistry.io/your-namespace"
  echo "  TAG       镜像标签，默认: latest"
  echo "  PUSH      是否构建后推送，默认: false"
  echo ""
  echo "Example:"
  echo "  REGISTRY=myregistry.io/your-namespace TAG=phase1 $0"
  echo "  REGISTRY=myregistry.io/your-namespace TAG=phase1 PUSH=true $0"
  exit 1
}

if [[ -z "${REGISTRY}" ]]; then
  usage
fi

# 去掉 REGISTRY 末尾的 /
REGISTRY="${REGISTRY%/}"
API_IMAGE="${REGISTRY}/liteboxd-server:${TAG}"
GATEWAY_IMAGE="${REGISTRY}/liteboxd-gateway:${TAG}"

echo "[Build] API image: ${API_IMAGE}"
docker build -f backend/Dockerfile -t "${API_IMAGE}" ./backend

echo "[Build] Gateway image: ${GATEWAY_IMAGE}"
docker build -f backend/Dockerfile.gateway -t "${GATEWAY_IMAGE}" ./backend

if [[ "${PUSH}" == "true" || "${PUSH}" == "1" ]]; then
  echo "[Push] ${API_IMAGE}"
  docker push "${API_IMAGE}"
  echo "[Push] ${GATEWAY_IMAGE}"
  docker push "${GATEWAY_IMAGE}"
fi

echo "[Build] Done. Images: ${API_IMAGE}, ${GATEWAY_IMAGE}"
echo "Deploy with: REGISTRY=${REGISTRY} TAG=${TAG} ./deploy/scripts/deploy-k8s.sh"
