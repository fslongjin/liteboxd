"""Async prepull service for LiteBoxd SDK."""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING, Any

from ..exceptions import PrepullFailedError
from ..exceptions import TimeoutError as LiteBoxdTimeoutError
from ..models import PrepullStatus, PrepullTask

if TYPE_CHECKING:
    from ..async_client import AsyncClient


class AsyncPrepullService:
    """Async image prepull operations service."""

    def __init__(self, client: "AsyncClient") -> None:
        """Initialize async prepull service."""
        self._client = client

    async def create(
        self,
        image: str,
        *,
        timeout: int = 600,
    ) -> PrepullTask:
        """Create a new prepull task."""
        payload: dict[str, Any] = {
            "image": image,
            "timeout": timeout,
        }
        response = await self._client._post("images/prepull", json=payload)
        return PrepullTask.model_validate(response.json())

    async def create_for_template(self, template_name: str) -> PrepullTask:
        """Create a prepull task for a template's image."""
        response = await self._client._post(f"templates/{template_name}/prepull")
        return PrepullTask.model_validate(response.json())

    async def list(
        self,
        *,
        image: str | None = None,
        status: PrepullStatus | None = None,
    ) -> list[PrepullTask]:
        """List prepull tasks."""
        params: dict[str, Any] = {}
        if image:
            params["image"] = image
        if status:
            params["status"] = status.value

        response = await self._client._get("images/prepull", params=params)
        data = response.json()
        return [PrepullTask.model_validate(item) for item in data.get("items", [])]

    async def get(self, task_id: str) -> PrepullTask:
        """Get a specific prepull task."""
        response = await self._client._get(f"images/prepull/{task_id}")
        return PrepullTask.model_validate(response.json())

    async def delete(self, task_id: str) -> None:
        """Delete a prepull task."""
        await self._client._delete(f"images/prepull/{task_id}")

    async def wait_for_completion(
        self,
        task_id: str,
        *,
        poll_interval: float = 5.0,
        timeout: float = 1800.0,
    ) -> PrepullTask:
        """Wait for prepull task to complete."""
        deadline = asyncio.get_event_loop().time() + timeout

        while True:
            task = await self.get(task_id)

            if task.status == PrepullStatus.COMPLETED:
                return task

            if task.status == PrepullStatus.FAILED:
                raise PrepullFailedError(task_id, task.error or "Unknown error")

            # Check timeout
            if asyncio.get_event_loop().time() >= deadline:
                raise LiteBoxdTimeoutError(
                    f"Timeout waiting for prepull task {task_id} to complete "
                    f"(current status: {task.status})"
                )

            # Wait before next poll
            await asyncio.sleep(poll_interval)
