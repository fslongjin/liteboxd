"""Tests for sandbox service."""

import pytest
from pytest_httpx import HTTPXMock

from liteboxd import Client, NotFoundError, SandboxOverrides, SandboxStatus


class TestSandboxService:
    """Tests for SandboxService."""

    def test_create_sandbox(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test creating a sandbox."""
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/sandboxes",
            json=sandbox_data,
            status_code=201,
        )

        with Client(base_url) as client:
            sandbox = client.sandbox.create(template="python-data-science")
            assert sandbox.id == sandbox_data["id"]
            assert sandbox.template == "python-data-science"

    def test_create_sandbox_with_overrides(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test creating a sandbox with overrides."""
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/sandboxes",
            json=sandbox_data,
            status_code=201,
        )

        with Client(base_url) as client:
            overrides = SandboxOverrides(
                cpu="1000m",
                memory="1Gi",
                ttl=7200,
                env={"DEBUG": "true"},
            )
            sandbox = client.sandbox.create(
                template="python-data-science",
                overrides=overrides,
            )
            assert sandbox.id == sandbox_data["id"]

    def test_list_sandboxes(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test listing sandboxes."""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/sandboxes",
            json={"items": [sandbox_data]},
        )

        with Client(base_url) as client:
            sandboxes = client.sandbox.list()
            assert len(sandboxes) == 1
            assert sandboxes[0].id == sandbox_data["id"]

    def test_get_sandbox(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test getting a sandbox."""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/sandboxes/{sandbox_data['id']}",
            json=sandbox_data,
        )

        with Client(base_url) as client:
            sandbox = client.sandbox.get(sandbox_data["id"])
            assert sandbox.id == sandbox_data["id"]
            assert sandbox.status == SandboxStatus.RUNNING

    def test_get_sandbox_not_found(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test getting a non-existent sandbox."""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/sandboxes/nonexistent",
            json={"error": "sandbox not found"},
            status_code=404,
        )

        with Client(base_url) as client:
            with pytest.raises(NotFoundError):
                client.sandbox.get("nonexistent")

    def test_delete_sandbox(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test deleting a sandbox."""
        httpx_mock.add_response(
            method="DELETE",
            url=f"{base_url}/sandboxes/{sandbox_data['id']}",
            status_code=204,
        )

        with Client(base_url) as client:
            client.sandbox.delete(sandbox_data["id"])

    def test_execute_command(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
        exec_result_data: dict,
    ) -> None:
        """Test executing a command in sandbox."""
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/sandboxes/{sandbox_data['id']}/exec",
            json=exec_result_data,
        )

        with Client(base_url) as client:
            result = client.sandbox.execute(
                sandbox_data["id"],
                command=["python", "-c", "print('Hello, World!')"],
            )
            assert result.exit_code == 0
            assert result.stdout == "Hello, World!\n"
            assert result.stderr == ""

    def test_get_logs(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test getting sandbox logs."""
        logs_data = {
            "logs": "Application started\n",
            "events": ["[Normal] Scheduled: Successfully assigned"],
        }
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/sandboxes/{sandbox_data['id']}/logs",
            json=logs_data,
        )

        with Client(base_url) as client:
            result = client.sandbox.get_logs(sandbox_data["id"])
            assert "Application started" in result.logs
            assert len(result.events) == 1

    def test_upload_file(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test uploading a file to sandbox."""
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/sandboxes/{sandbox_data['id']}/files",
            json={"message": "file uploaded successfully"},
        )

        with Client(base_url) as client:
            client.sandbox.upload_file(
                sandbox_data["id"],
                path="/workspace/test.py",
                content=b"print('hello')",
            )

    def test_download_file(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test downloading a file from sandbox."""
        file_content = b"print('hello')"
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/sandboxes/{sandbox_data['id']}/files?path=%2Fworkspace%2Ftest.py",
            content=file_content,
        )

        with Client(base_url) as client:
            content = client.sandbox.download_file(
                sandbox_data["id"],
                path="/workspace/test.py",
            )
            assert content == file_content
