"""Import/Export service for LiteBoxd SDK."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from ..models import ImportResult, ImportStrategy

if TYPE_CHECKING:
    from ..client import Client


class ImportExportService:
    """Template import/export operations service."""

    def __init__(self, client: "Client") -> None:
        """Initialize import/export service."""
        self._client = client

    def import_templates(
        self,
        yaml_content: bytes | str,
        *,
        strategy: ImportStrategy = ImportStrategy.CREATE_OR_UPDATE,
        auto_prepull: bool = False,
    ) -> ImportResult:
        """
        Import templates from YAML.

        Args:
            yaml_content: YAML content (bytes or string)
            strategy: Import strategy (create-only, update-only, create-or-update)
            auto_prepull: Whether to auto-prepull images after import

        Returns:
            Import result with details of each template

        Raises:
            BadRequestError: Invalid YAML or request
        """
        if isinstance(yaml_content, str):
            yaml_content = yaml_content.encode("utf-8")

        files = {
            "file": ("templates.yaml", yaml_content, "application/x-yaml"),
        }
        data: dict[str, Any] = {
            "strategy": strategy.value,
        }
        if auto_prepull:
            data["prepull"] = "true"

        response = self._client._post("templates/import", data=data, files=files)
        return ImportResult.model_validate(response.json())

    def export_all(
        self,
        *,
        tag: str | None = None,
        names: list[str] | None = None,
    ) -> bytes:
        """
        Export all templates as YAML.

        Args:
            tag: Filter by tag
            names: Filter by template names

        Returns:
            YAML content as bytes
        """
        params: dict[str, Any] = {}
        if tag:
            params["tag"] = tag
        if names:
            params["names"] = ",".join(names)

        response = self._client._get("templates/export", params=params)
        return response.content
