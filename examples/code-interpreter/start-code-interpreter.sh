#!/usr/bin/env bash
set -e

if [ -n "${WORKSPACE_DIR}" ]; then
  mkdir -p "${WORKSPACE_DIR}"
fi

if [ -z "${JUPYTER_TOKEN}" ]; then
  JUPYTER_TOKEN="$(
    /opt/venv/bin/python - <<'PY'
import secrets
print(secrets.token_urlsafe(16))
PY
  )"
fi

/opt/venv/bin/python -m jupyter notebook \
  --ip="${JUPYTER_HOST}" \
  --port="${JUPYTER_PORT}" \
  --no-browser \
  --NotebookApp.token="${JUPYTER_TOKEN}" \
  --NotebookApp.allow_origin='*' \
  --NotebookApp.allow_remote_access=true \
  --NotebookApp.notebook_dir="${WORKSPACE_DIR}" &

exec /opt/venv/bin/python /opt/liteboxd/bin/runtime_server.py
