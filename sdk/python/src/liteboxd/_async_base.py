"""Async base HTTP client for LiteBoxd SDK."""

from __future__ import annotations

from typing import Any

import httpx

from .exceptions import raise_for_status


class AsyncBaseClient:
    """Async base HTTP client with common request methods."""

    DEFAULT_TIMEOUT = 30.0
    USER_AGENT = "liteboxd-python-sdk/0.1.0"

    def __init__(
        self,
        base_url: str,
        *,
        timeout: float | httpx.Timeout = DEFAULT_TIMEOUT,
        auth_token: str | None = None,
        headers: dict[str, str] | None = None,
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        """
        Initialize the async base client.

        Args:
            base_url: API base URL (e.g., "http://localhost:8080/api/v1")
            timeout: Request timeout in seconds or httpx.Timeout object
            auth_token: Optional authentication token
            headers: Optional additional headers
            http_client: Optional custom httpx.AsyncClient instance
        """
        # Ensure base_url doesn't end with /
        self._base_url = base_url.rstrip("/")
        self._auth_token = auth_token

        # Build default headers
        self._headers = {
            "User-Agent": self.USER_AGENT,
            "Accept": "application/json",
        }
        if headers:
            self._headers.update(headers)
        if auth_token:
            self._headers["Authorization"] = f"Bearer {auth_token}"

        # Create or use provided HTTP client
        if http_client is not None:
            self._client = http_client
            self._owns_client = False
        else:
            self._client = httpx.AsyncClient(
                base_url=self._base_url,
                timeout=timeout,
                headers=self._headers,
            )
            self._owns_client = True

    def _build_url(self, path: str) -> str:
        """Build full URL from path."""
        return f"{self._base_url}/{path.lstrip('/')}"

    def _handle_response(self, response: httpx.Response) -> None:
        """Handle response and raise appropriate exceptions."""
        if response.status_code >= 400:
            # Try to extract error message from response body
            try:
                data = response.json()
                message = data.get("error", response.text)
            except Exception:
                message = response.text or response.reason_phrase
            raise_for_status(response.status_code, message)

    async def _request(
        self,
        method: str,
        path: str,
        *,
        json: Any | None = None,
        params: dict[str, Any] | None = None,
        data: dict[str, Any] | None = None,
        files: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """
        Make an async HTTP request.

        Args:
            method: HTTP method (GET, POST, PUT, DELETE, etc.)
            path: API path (e.g., "sandboxes")
            json: JSON body data
            params: Query parameters
            data: Form data
            files: Files for multipart upload
            headers: Additional headers for this request

        Returns:
            httpx.Response object
        """
        url = self._build_url(path)

        # Merge headers
        request_headers = self._headers.copy()
        if headers:
            request_headers.update(headers)

        # Don't set Content-Type for JSON, httpx does it automatically
        if json is not None:
            request_headers["Content-Type"] = "application/json"

        response = await self._client.request(
            method=method,
            url=url,
            json=json,
            params=params,
            data=data,
            files=files,
            headers=request_headers if headers else None,
        )

        self._handle_response(response)
        return response

    async def _get(
        self,
        path: str,
        *,
        params: dict[str, Any] | None = None,
    ) -> httpx.Response:
        """Make an async GET request."""
        return await self._request("GET", path, params=params)

    async def _post(
        self,
        path: str,
        *,
        json: Any | None = None,
        data: dict[str, Any] | None = None,
        files: dict[str, Any] | None = None,
    ) -> httpx.Response:
        """Make an async POST request."""
        return await self._request("POST", path, json=json, data=data, files=files)

    async def _put(
        self,
        path: str,
        *,
        json: Any | None = None,
    ) -> httpx.Response:
        """Make an async PUT request."""
        return await self._request("PUT", path, json=json)

    async def _delete(
        self,
        path: str,
        *,
        params: dict[str, Any] | None = None,
    ) -> httpx.Response:
        """Make an async DELETE request."""
        return await self._request("DELETE", path, params=params)

    async def close(self) -> None:
        """Close the HTTP client connection."""
        if self._owns_client:
            await self._client.aclose()

    async def __aenter__(self) -> "AsyncBaseClient":
        """Enter async context manager."""
        return self

    async def __aexit__(self, *args: Any) -> None:
        """Exit async context manager."""
        await self.close()
