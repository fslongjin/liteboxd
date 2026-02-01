"""Main client for LiteBoxd SDK."""

from __future__ import annotations

from typing import Any

import httpx

from ._base import BaseClient
from .services import ImportExportService, PrepullService, SandboxService, TemplateService


class Client(BaseClient):
    """
    LiteBoxd API client.

    This is the main entry point for interacting with the LiteBoxd API.
    It provides access to all service clients for managing sandboxes,
    templates, prepull tasks, and import/export operations.

    Example:
        >>> from liteboxd import Client
        >>>
        >>> # Basic usage
        >>> client = Client("http://localhost:8080/api/v1")
        >>> sandbox = client.sandbox.create(template="python-data-science")
        >>>
        >>> # With context manager
        >>> with Client("http://localhost:8080/api/v1") as client:
        ...     sandbox = client.sandbox.create(template="python-data-science")
        ...     result = client.sandbox.execute(sandbox.id, ["python", "-c", "print('hello')"])
        ...     print(result.stdout)
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8080/api/v1",
        *,
        timeout: float | httpx.Timeout = 30.0,
        auth_token: str | None = None,
        headers: dict[str, str] | None = None,
        http_client: httpx.Client | None = None,
    ) -> None:
        """
        Initialize the LiteBoxd client.

        Args:
            base_url: API base URL (default: http://localhost:8080/api/v1)
            timeout: Request timeout in seconds or httpx.Timeout object (default: 30.0)
            auth_token: Optional authentication token
            headers: Optional additional HTTP headers
            http_client: Optional custom httpx.Client instance
        """
        super().__init__(
            base_url,
            timeout=timeout,
            auth_token=auth_token,
            headers=headers,
            http_client=http_client,
        )

        # Initialize service clients
        self._sandbox = SandboxService(self)
        self._template = TemplateService(self)
        self._prepull = PrepullService(self)
        self._import_export = ImportExportService(self)

    @property
    def sandbox(self) -> SandboxService:
        """
        Sandbox operations service.

        Provides methods for creating, listing, managing sandboxes,
        executing commands, and handling files.
        """
        return self._sandbox

    @property
    def template(self) -> TemplateService:
        """
        Template operations service.

        Provides methods for creating, listing, updating, and managing
        template versions.
        """
        return self._template

    @property
    def prepull(self) -> PrepullService:
        """
        Image prepull operations service.

        Provides methods for managing container image prepull tasks.
        """
        return self._prepull

    @property
    def import_export(self) -> ImportExportService:
        """
        Template import/export service.

        Provides methods for importing and exporting templates as YAML.
        """
        return self._import_export

    def __enter__(self) -> "Client":
        """Enter context manager."""
        return self

    def __exit__(self, *args: Any) -> None:
        """Exit context manager."""
        self.close()
