"""Prepull-related models for LiteBoxd SDK."""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Optional

from pydantic import Field

from .common import LiteBoxdModel


class PrepullStatus(str, Enum):
    """Prepull task status."""

    PENDING = "pending"
    PULLING = "pulling"
    COMPLETED = "completed"
    FAILED = "failed"


class PrepullProgress(LiteBoxdModel):
    """Prepull progress information."""

    ready: int
    total: int


class PrepullTask(LiteBoxdModel):
    """Prepull task object."""

    id: str
    image: str
    status: PrepullStatus
    progress: Optional[PrepullProgress] = None
    template: Optional[str] = None
    error: Optional[str] = None
    started_at: datetime = Field(alias="startedAt")
    completed_at: Optional[datetime] = Field(default=None, alias="completedAt")


class ImportStrategy(str, Enum):
    """Import strategy for templates."""

    CREATE_ONLY = "create-only"
    UPDATE_ONLY = "update-only"
    CREATE_OR_UPDATE = "create-or-update"


class ImportAction(str, Enum):
    """Action performed on imported template."""

    CREATED = "created"
    UPDATED = "updated"
    SKIPPED = "skipped"
    FAILED = "failed"


class ImportResultItem(LiteBoxdModel):
    """Single template import result."""

    name: str
    action: ImportAction
    version: Optional[int] = None
    error: Optional[str] = None


class ImportResult(LiteBoxdModel):
    """Import operation result."""

    total: int
    created: int
    updated: int
    skipped: int
    failed: int
    results: list[ImportResultItem]
    prepull_started: Optional[list[str]] = Field(default=None, alias="prepullStarted")
