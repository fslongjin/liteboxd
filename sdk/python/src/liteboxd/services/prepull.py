"""Prepull service for LiteBoxd SDK."""

from __future__ import annotations

import time
from typing import TYPE_CHECKING, Any

from ..exceptions import PrepullFailedError
from ..exceptions import TimeoutError as LiteBoxdTimeoutError
from ..models import PrepullStatus, PrepullTask

if TYPE_CHECKING:
    from ..client import Client


class PrepullService:
    """Image prepull operations service."""

    def __init__(self, client: "Client") -> None:
        """Initialize prepull service."""
        self._client = client

    def create(
        self,
        image: str,
        *,
        timeout: int = 600,
    ) -> PrepullTask:
        """
        Create a new prepull task.

        Args:
            image: Container image to prepull
            timeout: Prepull timeout in seconds (default: 600)

        Returns:
            Created prepull task

        Raises:
            ConflictError: Prepull already in progress for this image
        """
        payload: dict[str, Any] = {
            "image": image,
            "timeout": timeout,
        }
        response = self._client._post("images/prepull", json=payload)
        return PrepullTask.model_validate(response.json())

    def create_for_template(self, template_name: str) -> PrepullTask:
        """
        Create a prepull task for a template's image.

        Args:
            template_name: Template name

        Returns:
            Created prepull task

        Raises:
            NotFoundError: Template not found
            ConflictError: Prepull already in progress
        """
        response = self._client._post(f"templates/{template_name}/prepull")
        return PrepullTask.model_validate(response.json())

    def list(
        self,
        *,
        image: str | None = None,
        status: PrepullStatus | None = None,
    ) -> list[PrepullTask]:
        """
        List prepull tasks.

        Args:
            image: Filter by image name
            status: Filter by status

        Returns:
            List of prepull tasks
        """
        params: dict[str, Any] = {}
        if image:
            params["image"] = image
        if status:
            params["status"] = status.value

        response = self._client._get("images/prepull", params=params)
        data = response.json()
        return [PrepullTask.model_validate(item) for item in data.get("items", [])]

    def get(self, task_id: str) -> PrepullTask:
        """
        Get a specific prepull task.

        Args:
            task_id: Prepull task ID

        Returns:
            Prepull task object

        Raises:
            NotFoundError: Task not found
        """
        response = self._client._get(f"images/prepull/{task_id}")
        return PrepullTask.model_validate(response.json())

    def delete(self, task_id: str) -> None:
        """
        Delete a prepull task.

        Args:
            task_id: Prepull task ID

        Raises:
            NotFoundError: Task not found
        """
        self._client._delete(f"images/prepull/{task_id}")

    def wait_for_completion(
        self,
        task_id: str,
        *,
        poll_interval: float = 5.0,
        timeout: float = 1800.0,
    ) -> PrepullTask:
        """
        Wait for prepull task to complete.

        Args:
            task_id: Prepull task ID
            poll_interval: Time between checks in seconds (default: 5.0)
            timeout: Maximum wait time in seconds (default: 1800.0)

        Returns:
            Completed prepull task

        Raises:
            TimeoutError: Wait timed out
            PrepullFailedError: Prepull task failed
        """
        start_time = time.monotonic()
        deadline = start_time + timeout

        while True:
            task = self.get(task_id)

            if task.status == PrepullStatus.COMPLETED:
                return task

            if task.status == PrepullStatus.FAILED:
                raise PrepullFailedError(task_id, task.error or "Unknown error")

            # Check timeout
            if time.monotonic() >= deadline:
                raise LiteBoxdTimeoutError(
                    f"Timeout waiting for prepull task {task_id} to complete "
                    f"(current status: {task.status})"
                )

            # Wait before next poll
            time.sleep(poll_interval)
