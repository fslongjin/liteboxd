"""Sandbox-related models for LiteBoxd SDK."""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Optional

from pydantic import Field

from .common import LiteBoxdModel


class SandboxStatus(str, Enum):
    """Sandbox status enumeration."""

    PENDING = "pending"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    TERMINATING = "terminating"
    UNKNOWN = "unknown"


class SandboxOverrides(LiteBoxdModel):
    """Sandbox configuration overrides."""

    cpu: Optional[str] = None
    memory: Optional[str] = None
    ttl: Optional[int] = None
    env: Optional[dict[str, str]] = None


class Sandbox(LiteBoxdModel):
    """Sandbox object."""

    id: str
    image: str
    cpu: str
    memory: str
    ttl: int
    env: Optional[dict[str, str]] = None
    status: SandboxStatus
    template: Optional[str] = None
    template_version: Optional[int] = Field(default=None, alias="templateVersion")
    created_at: datetime = Field(alias="created_at")
    expires_at: datetime = Field(alias="expires_at")
    access_token: Optional[str] = Field(default=None, alias="accessToken")
    access_url: Optional[str] = Field(default=None, alias="accessUrl")


class CreateSandboxRequest(LiteBoxdModel):
    """Request to create a sandbox."""

    template: str
    template_version: Optional[int] = Field(default=None, alias="templateVersion")
    overrides: Optional[SandboxOverrides] = None


class ExecResult(LiteBoxdModel):
    """Command execution result."""

    exit_code: int = Field(alias="exit_code")
    stdout: str
    stderr: str


class LogsResult(LiteBoxdModel):
    """Logs query result."""

    logs: str
    events: list[str]
