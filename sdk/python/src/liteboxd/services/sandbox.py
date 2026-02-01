"""Sandbox service for LiteBoxd SDK."""

from __future__ import annotations

import builtins
import time
from typing import TYPE_CHECKING, Any

from ..exceptions import SandboxFailedError
from ..exceptions import TimeoutError as LiteBoxdTimeoutError
from ..models import (
    CreateSandboxRequest,
    ExecResult,
    LogsResult,
    Sandbox,
    SandboxOverrides,
    SandboxStatus,
    to_api_dict,
)

if TYPE_CHECKING:
    from ..client import Client


class SandboxService:
    """Sandbox operations service."""

    def __init__(self, client: "Client") -> None:
        """Initialize sandbox service."""
        self._client = client

    def create(
        self,
        template: str,
        *,
        template_version: int | None = None,
        overrides: SandboxOverrides | None = None,
    ) -> Sandbox:
        """
        Create a sandbox from a template.

        Args:
            template: Template name (required)
            template_version: Template version (optional, defaults to latest)
            overrides: Configuration overrides (cpu, memory, ttl, env)

        Returns:
            Created sandbox object

        Raises:
            NotFoundError: Template not found
            BadRequestError: Invalid request parameters
        """
        request = CreateSandboxRequest(
            template=template,
            template_version=template_version,
            overrides=overrides,
        )
        response = self._client._post("sandboxes", json=to_api_dict(request))
        return Sandbox.model_validate(response.json())

    def list(self) -> builtins.list[Sandbox]:
        """
        List all sandboxes.

        Returns:
            List of sandbox objects
        """
        response = self._client._get("sandboxes")
        data = response.json()
        return [Sandbox.model_validate(item) for item in data.get("items", [])]

    def get(self, sandbox_id: str) -> Sandbox:
        """
        Get a specific sandbox by ID.

        Args:
            sandbox_id: Sandbox ID

        Returns:
            Sandbox object

        Raises:
            NotFoundError: Sandbox not found
        """
        response = self._client._get(f"sandboxes/{sandbox_id}")
        return Sandbox.model_validate(response.json())

    def delete(self, sandbox_id: str) -> None:
        """
        Delete a sandbox.

        Args:
            sandbox_id: Sandbox ID
        """
        self._client._delete(f"sandboxes/{sandbox_id}")

    def execute(
        self,
        sandbox_id: str,
        command: builtins.list[str],
        *,
        timeout: int = 30,
    ) -> ExecResult:
        """
        Execute a command in the sandbox.

        Args:
            sandbox_id: Sandbox ID
            command: Command to execute as list of strings
            timeout: Execution timeout in seconds

        Returns:
            Command execution result (exit_code, stdout, stderr)

        Raises:
            NotFoundError: Sandbox not found
        """
        payload: dict[str, Any] = {
            "command": command,
            "timeout": timeout,
        }
        response = self._client._post(f"sandboxes/{sandbox_id}/exec", json=payload)
        return ExecResult.model_validate(response.json())

    def get_logs(self, sandbox_id: str) -> LogsResult:
        """
        Get sandbox logs and events.

        Args:
            sandbox_id: Sandbox ID

        Returns:
            Logs and events
        """
        response = self._client._get(f"sandboxes/{sandbox_id}/logs")
        return LogsResult.model_validate(response.json())

    def upload_file(
        self,
        sandbox_id: str,
        path: str,
        content: bytes,
        *,
        content_type: str = "application/octet-stream",
    ) -> None:
        """
        Upload a file to the sandbox.

        Args:
            sandbox_id: Sandbox ID
            path: Target path inside the container
            content: File content as bytes
            content_type: Content type (default: application/octet-stream)
        """
        files = {
            "file": ("file", content, content_type),
        }
        data = {
            "path": path,
        }
        self._client._post(f"sandboxes/{sandbox_id}/files", data=data, files=files)

    def download_file(self, sandbox_id: str, path: str) -> bytes:
        """
        Download a file from the sandbox.

        Args:
            sandbox_id: Sandbox ID
            path: File path inside the container

        Returns:
            File content as bytes
        """
        response = self._client._get(
            f"sandboxes/{sandbox_id}/files",
            params={"path": path},
        )
        return response.content

    def wait_for_ready(
        self,
        sandbox_id: str,
        *,
        poll_interval: float = 2.0,
        timeout: float = 300.0,
    ) -> Sandbox:
        """
        Wait until the sandbox reaches running status.

        Args:
            sandbox_id: Sandbox ID
            poll_interval: Time between checks in seconds (default: 2.0)
            timeout: Maximum wait time in seconds (default: 300.0)

        Returns:
            Sandbox object in running state

        Raises:
            TimeoutError: Wait timed out
            SandboxFailedError: Sandbox failed to start
        """
        start_time = time.monotonic()
        deadline = start_time + timeout

        while True:
            sandbox = self.get(sandbox_id)

            if sandbox.status == SandboxStatus.RUNNING:
                return sandbox

            if sandbox.status == SandboxStatus.FAILED:
                raise SandboxFailedError(sandbox_id, "Sandbox failed to start")

            if sandbox.status == SandboxStatus.SUCCEEDED:
                raise SandboxFailedError(sandbox_id, "Sandbox exited unexpectedly")

            # Check timeout
            if time.monotonic() >= deadline:
                raise LiteBoxdTimeoutError(
                    f"Timeout waiting for sandbox {sandbox_id} to be ready "
                    f"(current status: {sandbox.status})"
                )

            # Wait before next poll
            time.sleep(poll_interval)
