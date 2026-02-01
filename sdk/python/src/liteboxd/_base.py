"""Base HTTP client for LiteBoxd SDK."""

from __future__ import annotations

from typing import Any

import httpx

from .exceptions import raise_for_status


class BaseClient:
    """Base HTTP client with common request methods."""

    DEFAULT_TIMEOUT = 30.0
    USER_AGENT = "liteboxd-python-sdk/0.1.0"

    def __init__(
        self,
        base_url: str,
        *,
        timeout: float | httpx.Timeout = DEFAULT_TIMEOUT,
        auth_token: str | None = None,
        headers: dict[str, str] | None = None,
        http_client: httpx.Client | None = None,
    ) -> None:
        """
        Initialize the base client.

        Args:
            base_url: API base URL (e.g., "http://localhost:8080/api/v1")
            timeout: Request timeout in seconds or httpx.Timeout object
            auth_token: Optional authentication token
            headers: Optional additional headers
            http_client: Optional custom httpx.Client instance
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
            self._client = httpx.Client(
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

    def _request(
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
        Make an HTTP request.

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
        # For files/multipart, also let httpx handle it
        if json is not None:
            request_headers["Content-Type"] = "application/json"

        response = self._client.request(
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

    def _get(
        self,
        path: str,
        *,
        params: dict[str, Any] | None = None,
    ) -> httpx.Response:
        """Make a GET request."""
        return self._request("GET", path, params=params)

    def _post(
        self,
        path: str,
        *,
        json: Any | None = None,
        data: dict[str, Any] | None = None,
        files: dict[str, Any] | None = None,
    ) -> httpx.Response:
        """Make a POST request."""
        return self._request("POST", path, json=json, data=data, files=files)

    def _put(
        self,
        path: str,
        *,
        json: Any | None = None,
    ) -> httpx.Response:
        """Make a PUT request."""
        return self._request("PUT", path, json=json)

    def _delete(
        self,
        path: str,
        *,
        params: dict[str, Any] | None = None,
    ) -> httpx.Response:
        """Make a DELETE request."""
        return self._request("DELETE", path, params=params)

    def close(self) -> None:
        """Close the HTTP client connection."""
        if self._owns_client:
            self._client.close()

    def __enter__(self) -> "BaseClient":
        """Enter context manager."""
        return self

    def __exit__(self, *args: Any) -> None:
        """Exit context manager."""
        self.close()
