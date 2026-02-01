"""Common models for LiteBoxd SDK."""

from __future__ import annotations

from datetime import datetime
from typing import Any

from pydantic import BaseModel, ConfigDict


class LiteBoxdModel(BaseModel):
    """Base model for all LiteBoxd models."""

    model_config = ConfigDict(
        populate_by_name=True,
    )


def to_api_dict(model: BaseModel) -> dict[str, Any]:
    """Convert model to API-compatible dict (using aliases)."""
    return model.model_dump(by_alias=True, exclude_none=True)


def parse_datetime(value: str | datetime | None) -> datetime | None:
    """Parse datetime from string or return as-is."""
    if value is None:
        return None
    if isinstance(value, datetime):
        return value
    return datetime.fromisoformat(value.replace("Z", "+00:00"))
