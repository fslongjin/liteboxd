import base64
import json
import os
import subprocess
import tempfile
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

WORKSPACE_DIR = os.environ.get("WORKSPACE_DIR", "/workspace")
DEFAULT_TIMEOUT = 30


def resolve_path(raw_path: str) -> Path:
    workspace = Path(WORKSPACE_DIR).resolve()
    path = Path(raw_path)
    if path.is_absolute():
        target = path.resolve()
    else:
        target = (workspace / raw_path).resolve()
    if not str(target).startswith(str(workspace)):
        raise ValueError("invalid path")
    return target


def ensure_session_dir(session_id: str | None) -> Path:
    if not session_id:
        session_id = f"sess-{uuid.uuid4().hex}"
    session_root = resolve_path(".sessions")
    session_root.mkdir(parents=True, exist_ok=True)
    session_dir = resolve_path(f".sessions/{session_id}")
    session_dir.mkdir(parents=True, exist_ok=True)
    return session_dir


def run_command(command, cwd: Path, timeout: int):
    start = time.time()
    proc = subprocess.Popen(
        command,
        cwd=str(cwd),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    try:
        stdout, stderr = proc.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        proc.kill()
        stdout, stderr = proc.communicate()
        return {
            "exit_code": -1,
            "stdout": stdout,
            "stderr": stderr + "\nprocess timeout",
            "duration_ms": int((time.time() - start) * 1000),
        }
    return {
        "exit_code": proc.returncode,
        "stdout": stdout,
        "stderr": stderr,
        "duration_ms": int((time.time() - start) * 1000),
    }


def wrap_java(code: str) -> str:
    return "\n".join(
        [
            "public class Main {",
            "  public static void main(String[] args) throws Exception {",
            code,
            "  }",
            "}",
        ]
    )


def wrap_go(code: str) -> str:
    return "\n".join(
        [
            "package main",
            "import (",
            '  "fmt"',
            ")",
            "func main() {",
            code,
            "}",
        ]
    )


class Handler(BaseHTTPRequestHandler):
    def _send_json(self, status: int, payload: dict):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _read_json(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length == 0:
            return {}
        raw = self.rfile.read(length)
        return json.loads(raw.decode("utf-8"))

    def _parse_query(self):
        return parse_qs(urlparse(self.path).query)

    def do_GET(self):
        if self.path.startswith("/ping"):
            self._send_json(200, {"status": "ok"})
            return

        if self.path.startswith("/files/list"):
            query = self._parse_query()
            raw_path = (query.get("path") or ["/"])[0]
            try:
                target = resolve_path(raw_path)
                if not target.exists():
                    self._send_json(404, {"error": "path not found"})
                    return
                if not target.is_dir():
                    self._send_json(400, {"error": "path is not a directory"})
                    return
                items = []
                for entry in target.iterdir():
                    stat = entry.stat()
                    items.append(
                        {
                            "name": entry.name,
                            "path": str(entry.relative_to(resolve_path("."))),
                            "is_dir": entry.is_dir(),
                            "size": stat.st_size,
                            "modified_at": int(stat.st_mtime),
                        }
                    )
                self._send_json(200, {"items": items})
            except Exception as exc:
                self._send_json(400, {"error": str(exc)})
            return

        if self.path.startswith("/files/download"):
            query = self._parse_query()
            raw_path = (query.get("path") or [""])[0]
            try:
                target = resolve_path(raw_path)
                if not target.exists() or not target.is_file():
                    self._send_json(404, {"error": "file not found"})
                    return
                data = target.read_bytes()
                self.send_response(200)
                self.send_header("Content-Type", "application/octet-stream")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)
            except Exception as exc:
                self._send_json(400, {"error": str(exc)})
            return

        self._send_json(404, {"error": "not found"})

    def do_DELETE(self):
        if self.path.startswith("/sessions/"):
            session_id = self.path.split("/sessions/")[-1]
            try:
                session_dir = resolve_path(f".sessions/{session_id}")
                if session_dir.exists():
                    for item in session_dir.rglob("*"):
                        if item.is_file():
                            item.unlink()
                    for item in sorted(session_dir.rglob("*"), reverse=True):
                        if item.is_dir():
                            item.rmdir()
                    session_dir.rmdir()
                self._send_json(200, {"session_id": session_id})
            except Exception as exc:
                self._send_json(400, {"error": str(exc)})
            return
        self._send_json(404, {"error": "not found"})

    def do_POST(self):
        if self.path == "/sessions":
            session_id = f"sess-{uuid.uuid4().hex}"
            ensure_session_dir(session_id)
            self._send_json(200, {"session_id": session_id})
            return

        if self.path == "/files/write":
            payload = self._read_json()
            raw_path = payload.get("path", "")
            content = payload.get("content", "")
            encoding = payload.get("encoding", "utf-8")
            try:
                target = resolve_path(raw_path)
                target.parent.mkdir(parents=True, exist_ok=True)
                if encoding == "base64":
                    target.write_bytes(base64.b64decode(content.encode("utf-8")))
                else:
                    target.write_text(content, encoding="utf-8")
                self._send_json(200, {"path": str(target)})
            except Exception as exc:
                self._send_json(400, {"error": str(exc)})
            return

        if self.path == "/files/delete":
            payload = self._read_json()
            raw_path = payload.get("path", "")
            try:
                target = resolve_path(raw_path)
                if target.is_file():
                    target.unlink()
                elif target.is_dir():
                    for item in target.rglob("*"):
                        if item.is_file():
                            item.unlink()
                    for item in sorted(target.rglob("*"), reverse=True):
                        if item.is_dir():
                            item.rmdir()
                    target.rmdir()
                self._send_json(200, {"path": str(target)})
            except Exception as exc:
                self._send_json(400, {"error": str(exc)})
            return

        if self.path == "/command":
            payload = self._read_json()
            command = payload.get("command")
            timeout = int(payload.get("timeout", DEFAULT_TIMEOUT))
            session_id = payload.get("session_id")
            if not command:
                self._send_json(400, {"error": "missing command"})
                return
            if isinstance(command, str):
                cmd = ["sh", "-c", command]
            else:
                cmd = command
            try:
                session_dir = ensure_session_dir(session_id)
                result = run_command(cmd, session_dir, timeout)
                result["session_id"] = session_id
                self._send_json(200, result)
            except Exception as exc:
                self._send_json(500, {"error": str(exc)})
            return

        if self.path == "/code":
            payload = self._read_json()
            language = (payload.get("language") or "python").lower()
            code = payload.get("code") or ""
            timeout = int(payload.get("timeout", DEFAULT_TIMEOUT))
            session_id = payload.get("session_id")
            try:
                session_dir = ensure_session_dir(session_id)
                if language == "python":
                    filename = session_dir / "main.py"
                    filename.write_text(code, encoding="utf-8")
                    cmd = ["python3", str(filename)]
                elif language in {"javascript", "js"}:
                    filename = session_dir / "main.js"
                    filename.write_text(code, encoding="utf-8")
                    cmd = ["node", str(filename)]
                elif language in {"typescript", "ts"}:
                    filename = session_dir / "main.ts"
                    filename.write_text(code, encoding="utf-8")
                    cmd = ["ts-node", str(filename)]
                elif language == "bash":
                    filename = session_dir / "main.sh"
                    filename.write_text(code, encoding="utf-8")
                    cmd = ["bash", str(filename)]
                elif language == "go":
                    filename = session_dir / "main.go"
                    filename.write_text(wrap_go(code), encoding="utf-8")
                    cmd = ["go", "run", str(filename)]
                elif language == "java":
                    filename = session_dir / "Main.java"
                    filename.write_text(wrap_java(code), encoding="utf-8")
                    compile_result = run_command(["javac", str(filename)], session_dir, timeout)
                    if compile_result["exit_code"] != 0:
                        compile_result["session_id"] = session_id
                        self._send_json(200, compile_result)
                        return
                    cmd = ["java", "-cp", str(session_dir), "Main"]
                else:
                    self._send_json(400, {"error": "unsupported language"})
                    return
                result = run_command(cmd, session_dir, timeout)
                result["session_id"] = session_id
                self._send_json(200, result)
            except Exception as exc:
                self._send_json(500, {"error": str(exc)})
            return

        self._send_json(404, {"error": "not found"})


def main():
    host = os.environ.get("RUNTIME_HOST", "0.0.0.0")
    port = int(os.environ.get("RUNTIME_PORT", "44772"))
    server = ThreadingHTTPServer((host, port), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
