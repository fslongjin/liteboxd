"""Template service for LiteBoxd SDK."""

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
    from ..client import Client


class TemplateService:
    """Template operations service."""

    def __init__(self, client: "Client") -> None:
        """Initialize template service."""
        self._client = client

    def create(self, request: CreateTemplateRequest) -> Template:
        """
        Create a new template.

        Args:
            request: Template creation request

        Returns:
            Created template object

        Raises:
            ConflictError: Template already exists
            BadRequestError: Invalid request parameters
        """
        response = self._client._post("templates", json=to_api_dict(request))
        return Template.model_validate(response.json())

    def list(
        self,
        *,
        tag: str | None = None,
        search: str | None = None,
        page: int = 1,
        page_size: int = 20,
    ) -> TemplateListResult:
        """
        List templates with optional filtering.

        Args:
            tag: Filter by tag
            search: Search in name, displayName, description
            page: Page number (default: 1)
            page_size: Items per page (default: 20, max: 100)

        Returns:
            Template list result with pagination info
        """
        params: dict[str, Any] = {
            "page": page,
            "pageSize": page_size,
        }
        if tag:
            params["tag"] = tag
        if search:
            params["search"] = search

        response = self._client._get("templates", params=params)
        return TemplateListResult.model_validate(response.json())

    def get(self, name: str) -> Template:
        """
        Get a specific template by name.

        Args:
            name: Template name

        Returns:
            Template object

        Raises:
            NotFoundError: Template not found
        """
        response = self._client._get(f"templates/{name}")
        return Template.model_validate(response.json())

    def update(self, name: str, request: UpdateTemplateRequest) -> Template:
        """
        Update a template (creates a new version).

        Args:
            name: Template name
            request: Update request

        Returns:
            Updated template object

        Raises:
            NotFoundError: Template not found
        """
        response = self._client._put(f"templates/{name}", json=to_api_dict(request))
        return Template.model_validate(response.json())

    def delete(self, name: str) -> None:
        """
        Delete a template.

        Args:
            name: Template name

        Raises:
            NotFoundError: Template not found
        """
        self._client._delete(f"templates/{name}")

    def list_versions(self, name: str) -> VersionListResult:
        """
        List all versions of a template.

        Args:
            name: Template name

        Returns:
            Version list result

        Raises:
            NotFoundError: Template not found
        """
        response = self._client._get(f"templates/{name}/versions")
        return VersionListResult.model_validate(response.json())

    def get_version(self, name: str, version: int) -> TemplateVersion:
        """
        Get a specific version of a template.

        Args:
            name: Template name
            version: Version number

        Returns:
            Template version object

        Raises:
            NotFoundError: Template or version not found
        """
        response = self._client._get(f"templates/{name}/versions/{version}")
        return TemplateVersion.model_validate(response.json())

    def rollback(
        self,
        name: str,
        target_version: int,
        *,
        changelog: str | None = None,
    ) -> RollbackResult:
        """
        Rollback template to a specific version.

        Args:
            name: Template name
            target_version: Version to rollback to
            changelog: Optional changelog message

        Returns:
            Rollback result

        Raises:
            NotFoundError: Template or version not found
        """
        payload: dict[str, Any] = {
            "targetVersion": target_version,
        }
        if changelog:
            payload["changelog"] = changelog

        response = self._client._post(f"templates/{name}/rollback", json=payload)
        return RollbackResult.model_validate(response.json())

    def export_yaml(
        self,
        name: str,
        *,
        version: int | None = None,
    ) -> bytes:
        """
        Export a template as YAML.

        Args:
            name: Template name
            version: Specific version (optional, defaults to latest)

        Returns:
            YAML content as bytes

        Raises:
            NotFoundError: Template not found
        """
        params: dict[str, Any] = {}
        if version is not None:
            params["version"] = version

        response = self._client._get(f"templates/{name}/export", params=params)
        return response.content
