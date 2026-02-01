"""Tests for prepull service."""

import pytest
from pytest_httpx import HTTPXMock

from liteboxd import Client, PrepullStatus


class TestPrepullService:
    """Tests for PrepullService."""

    def test_create_prepull(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test creating a prepull task."""
        prepull_data = {
            "id": "pp-123",
            "image": "python:3.11-slim",
            "status": "pending",
            "progress": {"ready": 0, "total": 3},
            "startedAt": "2024-01-01T00:00:00Z",
        }
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/images/prepull",
            json=prepull_data,
            status_code=202,
        )

        with Client(base_url) as client:
            task = client.prepull.create("python:3.11-slim", timeout=600)
            assert task.id == "pp-123"
            assert task.status == PrepullStatus.PENDING

    def test_create_prepull_for_template(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test creating a prepull task for a template."""
        prepull_data = {
            "id": "pp-123",
            "image": "python:3.11-slim",
            "status": "pending",
            "template": "python-data-science",
            "startedAt": "2024-01-01T00:00:00Z",
        }
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/templates/python-data-science/prepull",
            json=prepull_data,
            status_code=202,
        )

        with Client(base_url) as client:
            task = client.prepull.create_for_template("python-data-science")
            assert task.id == "pp-123"
            assert task.template == "python-data-science"

    def test_list_prepull_tasks(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test listing prepull tasks."""
        prepull_data = {
            "items": [
                {
                    "id": "pp-123",
                    "image": "python:3.11-slim",
                    "status": "completed",
                    "startedAt": "2024-01-01T00:00:00Z",
                    "completedAt": "2024-01-01T00:05:00Z",
                }
            ]
        }
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/images/prepull",
            json=prepull_data,
        )

        with Client(base_url) as client:
            tasks = client.prepull.list()
            assert len(tasks) == 1
            assert tasks[0].status == PrepullStatus.COMPLETED

    def test_list_prepull_tasks_with_filter(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test listing prepull tasks with filters."""
        prepull_data = {"items": []}
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/images/prepull?status=pulling",
            json=prepull_data,
        )

        with Client(base_url) as client:
            tasks = client.prepull.list(status=PrepullStatus.PULLING)
            assert len(tasks) == 0

    def test_get_prepull_task(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test getting a prepull task."""
        prepull_data = {
            "id": "pp-123",
            "image": "python:3.11-slim",
            "status": "pulling",
            "progress": {"ready": 1, "total": 3},
            "startedAt": "2024-01-01T00:00:00Z",
        }
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/images/prepull/pp-123",
            json=prepull_data,
        )

        with Client(base_url) as client:
            task = client.prepull.get("pp-123")
            assert task.id == "pp-123"
            assert task.progress is not None
            assert task.progress.ready == 1
            assert task.progress.total == 3

    def test_delete_prepull_task(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test deleting a prepull task."""
        httpx_mock.add_response(
            method="DELETE",
            url=f"{base_url}/images/prepull/pp-123",
            status_code=204,
        )

        with Client(base_url) as client:
            client.prepull.delete("pp-123")
