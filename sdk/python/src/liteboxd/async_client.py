"""Async client for LiteBoxd SDK."""

from __future__ import annotations

from typing import Any

import httpx

from ._async_base import AsyncBaseClient
from .services.async_import_export import AsyncImportExportService
from .services.async_prepull import AsyncPrepullService
from .services.async_sandbox import AsyncSandboxService
from .services.async_template import AsyncTemplateService


class AsyncClient(AsyncBaseClient):
    """
    Async LiteBoxd API client.

    This is the async entry point for interacting with the LiteBoxd API.
    All methods are coroutines and must be awaited.

    Example:
        >>> import asyncio
        >>> from liteboxd import AsyncClient
        >>>
        >>> async def main():
        ...     async with AsyncClient("http://localhost:8080/api/v1") as client:
        ...         sandbox = await client.sandbox.create(template="python-data-science")
        ...         sandbox = await client.sandbox.wait_for_ready(sandbox.id)
        ...         result = await client.sandbox.execute(
        ...             sandbox.id, ["python", "-c", "print('hello')"]
        ...         )
        ...         print(result.stdout)
        ...         await client.sandbox.delete(sandbox.id)
        >>>
        >>> asyncio.run(main())
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8080/api/v1",
        *,
        timeout: float | httpx.Timeout = 30.0,
        auth_token: str | None = None,
        headers: dict[str, str] | None = None,
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        """
        Initialize the async LiteBoxd client.

        Args:
            base_url: API base URL (default: http://localhost:8080/api/v1)
            timeout: Request timeout in seconds or httpx.Timeout object (default: 30.0)
            auth_token: Optional authentication token
            headers: Optional additional HTTP headers
            http_client: Optional custom httpx.AsyncClient instance
        """
        super().__init__(
            base_url,
            timeout=timeout,
            auth_token=auth_token,
            headers=headers,
            http_client=http_client,
        )

        # Initialize async service clients
        self._sandbox = AsyncSandboxService(self)
        self._template = AsyncTemplateService(self)
        self._prepull = AsyncPrepullService(self)
        self._import_export = AsyncImportExportService(self)

    @property
    def sandbox(self) -> AsyncSandboxService:
        """Async sandbox operations service."""
        return self._sandbox

    @property
    def template(self) -> AsyncTemplateService:
        """Async template operations service."""
        return self._template

    @property
    def prepull(self) -> AsyncPrepullService:
        """Async image prepull operations service."""
        return self._prepull

    @property
    def import_export(self) -> AsyncImportExportService:
        """Async template import/export service."""
        return self._import_export

    async def __aenter__(self) -> "AsyncClient":
        """Enter async context manager."""
        return self

    async def __aexit__(self, *args: Any) -> None:
        """Exit async context manager."""
        await self.close()
