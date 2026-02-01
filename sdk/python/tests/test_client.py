"""Tests for the LiteBoxd client."""

import pytest
from pytest_httpx import HTTPXMock

from liteboxd import Client


class TestClient:
    """Tests for Client class."""

    def test_client_creation(self, base_url: str) -> None:
        """Test client creation with default settings."""
        client = Client(base_url)
        assert client._base_url == base_url
        client.close()

    def test_client_with_auth_token(self, base_url: str) -> None:
        """Test client creation with auth token."""
        client = Client(base_url, auth_token="test-token")
        assert client._auth_token == "test-token"
        assert "Authorization" in client._headers
        assert client._headers["Authorization"] == "Bearer test-token"
        client.close()

    def test_client_context_manager(self, base_url: str) -> None:
        """Test client as context manager."""
        with Client(base_url) as client:
            assert client is not None

    def test_client_services_available(self, base_url: str) -> None:
        """Test that all services are available."""
        with Client(base_url) as client:
            assert client.sandbox is not None
            assert client.template is not None
            assert client.prepull is not None
            assert client.import_export is not None


class TestClientRequests:
    """Tests for Client HTTP requests."""

    def test_get_request(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test GET request."""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/sandboxes/{sandbox_data['id']}",
            json=sandbox_data,
        )

        with Client(base_url) as client:
            sandbox = client.sandbox.get(sandbox_data["id"])
            assert sandbox.id == sandbox_data["id"]
            assert sandbox.status.value == sandbox_data["status"]

    def test_post_request(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test POST request."""
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/sandboxes",
            json=sandbox_data,
            status_code=201,
        )

        with Client(base_url) as client:
            sandbox = client.sandbox.create(template="python-data-science")
            assert sandbox.id == sandbox_data["id"]

    def test_delete_request(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        sandbox_data: dict,
    ) -> None:
        """Test DELETE request."""
        httpx_mock.add_response(
            method="DELETE",
            url=f"{base_url}/sandboxes/{sandbox_data['id']}",
            status_code=204,
        )

        with Client(base_url) as client:
            client.sandbox.delete(sandbox_data["id"])
