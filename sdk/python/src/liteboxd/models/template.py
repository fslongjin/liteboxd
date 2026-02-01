"""Template-related models for LiteBoxd SDK."""

from __future__ import annotations

from datetime import datetime
from typing import Optional

from pydantic import Field

from .common import LiteBoxdModel


class ResourceSpec(LiteBoxdModel):
    """Resource specification."""

    cpu: str = "500m"
    memory: str = "512Mi"


class FileSpec(LiteBoxdModel):
    """File specification."""

    source: Optional[str] = None
    destination: str
    content: Optional[str] = None


class ExecAction(LiteBoxdModel):
    """Exec action for probes."""

    command: list[str]


class ProbeSpec(LiteBoxdModel):
    """Probe specification."""

    exec: ExecAction
    initial_delay_seconds: int = Field(default=0, alias="initialDelaySeconds")
    period_seconds: int = Field(default=10, alias="periodSeconds")
    failure_threshold: int = Field(default=3, alias="failureThreshold")


class NetworkSpec(LiteBoxdModel):
    """Network configuration."""

    allow_internet_access: bool = Field(default=False, alias="allowInternetAccess")
    allowed_domains: Optional[list[str]] = Field(default=None, alias="allowedDomains")


class TemplateSpec(LiteBoxdModel):
    """Template specification."""

    image: str
    command: Optional[list[str]] = None
    args: Optional[list[str]] = None
    resources: ResourceSpec = Field(default_factory=ResourceSpec)
    ttl: int = 3600
    env: Optional[dict[str, str]] = None
    startup_script: Optional[str] = Field(default=None, alias="startupScript")
    startup_timeout: int = Field(default=300, alias="startupTimeout")
    files: Optional[list[FileSpec]] = None
    readiness_probe: Optional[ProbeSpec] = Field(default=None, alias="readinessProbe")
    network: Optional[NetworkSpec] = None


class Template(LiteBoxdModel):
    """Template object."""

    id: str
    name: str
    display_name: Optional[str] = Field(default=None, alias="displayName")
    description: Optional[str] = None
    tags: Optional[list[str]] = None
    author: Optional[str] = None
    is_public: bool = Field(default=True, alias="isPublic")
    latest_version: int = Field(alias="latestVersion")
    created_at: datetime = Field(alias="createdAt")
    updated_at: datetime = Field(alias="updatedAt")
    spec: Optional[TemplateSpec] = None


class TemplateVersion(LiteBoxdModel):
    """Template version."""

    id: str
    template_id: str = Field(alias="templateId")
    version: int
    spec: TemplateSpec
    changelog: Optional[str] = None
    created_by: Optional[str] = Field(default=None, alias="createdBy")
    created_at: datetime = Field(alias="createdAt")


class CreateTemplateRequest(LiteBoxdModel):
    """Request to create a template."""

    name: str
    display_name: Optional[str] = Field(default=None, alias="displayName")
    description: Optional[str] = None
    tags: Optional[list[str]] = None
    is_public: Optional[bool] = Field(default=None, alias="isPublic")
    spec: TemplateSpec
    auto_prepull: bool = Field(default=False, alias="autoPrepull")


class UpdateTemplateRequest(LiteBoxdModel):
    """Request to update a template."""

    display_name: Optional[str] = Field(default=None, alias="displayName")
    description: Optional[str] = None
    tags: Optional[list[str]] = None
    is_public: Optional[bool] = Field(default=None, alias="isPublic")
    spec: TemplateSpec
    changelog: Optional[str] = None


class RollbackResult(LiteBoxdModel):
    """Rollback operation result."""

    id: str
    name: str
    latest_version: int = Field(alias="latestVersion")
    rolled_back_from: int = Field(alias="rolledBackFrom")
    rolled_back_to: int = Field(alias="rolledBackTo")
    updated_at: datetime = Field(alias="updatedAt")


class TemplateListResult(LiteBoxdModel):
    """Template list result with pagination."""

    items: list[Template]
    total: int
    page: int
    page_size: int = Field(alias="pageSize")


class VersionListResult(LiteBoxdModel):
    """Version list result."""

    items: list[TemplateVersion]
    total: int
