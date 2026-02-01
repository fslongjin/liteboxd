"""Async template service for LiteBoxd SDK."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from ..models import (
    CreateTemplateRequest,
    RollbackResult,
    Template,
    TemplateListResult,
    TemplateVersion,
    UpdateTemplateRequest,
    VersionListResult,
    to_api_dict,
)

if TYPE_CHECKING:
    from ..async_client import AsyncClient


class AsyncTemplateService:
    """Async template operations service."""

    def __init__(self, client: "AsyncClient") -> None:
        """Initialize async template service."""
        self._client = client

    async def create(self, request: CreateTemplateRequest) -> Template:
        """Create a new template."""
        response = await self._client._post("templates", json=to_api_dict(request))
        return Template.model_validate(response.json())

    async def list(
        self,
        *,
        tag: str | None = None,
        search: str | None = None,
        page: int = 1,
        page_size: int = 20,
    ) -> TemplateListResult:
        """List templates with optional filtering."""
        params: dict[str, Any] = {
            "page": page,
            "pageSize": page_size,
        }
        if tag:
            params["tag"] = tag
        if search:
            params["search"] = search

        response = await self._client._get("templates", params=params)
        return TemplateListResult.model_validate(response.json())

    async def get(self, name: str) -> Template:
        """Get a specific template by name."""
        response = await self._client._get(f"templates/{name}")
        return Template.model_validate(response.json())

    async def update(self, name: str, request: UpdateTemplateRequest) -> Template:
        """Update a template (creates a new version)."""
        response = await self._client._put(f"templates/{name}", json=to_api_dict(request))
        return Template.model_validate(response.json())

    async def delete(self, name: str) -> None:
        """Delete a template."""
        await self._client._delete(f"templates/{name}")

    async def list_versions(self, name: str) -> VersionListResult:
        """List all versions of a template."""
        response = await self._client._get(f"templates/{name}/versions")
        return VersionListResult.model_validate(response.json())

    async def get_version(self, name: str, version: int) -> TemplateVersion:
        """Get a specific version of a template."""
        response = await self._client._get(f"templates/{name}/versions/{version}")
        return TemplateVersion.model_validate(response.json())

    async def rollback(
        self,
        name: str,
        target_version: int,
        *,
        changelog: str | None = None,
    ) -> RollbackResult:
        """Rollback template to a specific version."""
        payload: dict[str, Any] = {
            "targetVersion": target_version,
        }
        if changelog:
            payload["changelog"] = changelog

        response = await self._client._post(f"templates/{name}/rollback", json=payload)
        return RollbackResult.model_validate(response.json())

    async def export_yaml(
        self,
        name: str,
        *,
        version: int | None = None,
    ) -> bytes:
        """Export a template as YAML."""
        params: dict[str, Any] = {}
        if version is not None:
            params["version"] = version

        response = await self._client._get(f"templates/{name}/export", params=params)
        return response.content
