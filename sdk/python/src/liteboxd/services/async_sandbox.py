"""Async sandbox service for LiteBoxd SDK."""

from __future__ import annotations

import asyncio
import builtins
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
    from ..async_client import AsyncClient


class AsyncSandboxService:
    """Async sandbox operations service."""

    def __init__(self, client: "AsyncClient") -> None:
        """Initialize async sandbox service."""
        self._client = client

    async def create(
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
        """
        request = CreateSandboxRequest(
            template=template,
            template_version=template_version,
            overrides=overrides,
        )
        response = await self._client._post("sandboxes", json=to_api_dict(request))
        return Sandbox.model_validate(response.json())

    async def list(self) -> builtins.list[Sandbox]:
        """List all sandboxes."""
        response = await self._client._get("sandboxes")
        data = response.json()
        return [Sandbox.model_validate(item) for item in data.get("items", [])]

    async def get(self, sandbox_id: str) -> Sandbox:
        """Get a specific sandbox by ID."""
        response = await self._client._get(f"sandboxes/{sandbox_id}")
        return Sandbox.model_validate(response.json())

    async def delete(self, sandbox_id: str) -> None:
        """Delete a sandbox."""
        await self._client._delete(f"sandboxes/{sandbox_id}")

    async def execute(
        self,
        sandbox_id: str,
        command: builtins.list[str],
        *,
        timeout: int = 30,
    ) -> ExecResult:
        """Execute a command in the sandbox."""
        payload: dict[str, Any] = {
            "command": command,
            "timeout": timeout,
        }
        response = await self._client._post(f"sandboxes/{sandbox_id}/exec", json=payload)
        return ExecResult.model_validate(response.json())

    async def get_logs(self, sandbox_id: str) -> LogsResult:
        """Get sandbox logs and events."""
        response = await self._client._get(f"sandboxes/{sandbox_id}/logs")
        return LogsResult.model_validate(response.json())

    async def upload_file(
        self,
        sandbox_id: str,
        path: str,
        content: bytes,
        *,
        content_type: str = "application/octet-stream",
    ) -> None:
        """Upload a file to the sandbox."""
        files = {
            "file": ("file", content, content_type),
        }
        data = {
            "path": path,
        }
        await self._client._post(f"sandboxes/{sandbox_id}/files", data=data, files=files)

    async def download_file(self, sandbox_id: str, path: str) -> bytes:
        """Download a file from the sandbox."""
        response = await self._client._get(
            f"sandboxes/{sandbox_id}/files",
            params={"path": path},
        )
        return response.content

    async def wait_for_ready(
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
        deadline = asyncio.get_event_loop().time() + timeout

        while True:
            sandbox = await self.get(sandbox_id)

            if sandbox.status == SandboxStatus.RUNNING:
                return sandbox

            if sandbox.status == SandboxStatus.FAILED:
                raise SandboxFailedError(sandbox_id, "Sandbox failed to start")

            if sandbox.status == SandboxStatus.SUCCEEDED:
                raise SandboxFailedError(sandbox_id, "Sandbox exited unexpectedly")

            # Check timeout
            if asyncio.get_event_loop().time() >= deadline:
                raise LiteBoxdTimeoutError(
                    f"Timeout waiting for sandbox {sandbox_id} to be ready "
                    f"(current status: {sandbox.status})"
                )

            # Wait before next poll
            await asyncio.sleep(poll_interval)
