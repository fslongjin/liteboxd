"""
LiteBoxd Python SDK.

A Python SDK for interacting with the LiteBoxd API - a lightweight K8s sandbox system.

Example:
    >>> from liteboxd import Client
    >>>
    >>> with Client("http://localhost:8080/api/v1") as client:
    ...     # Create a sandbox from a template
    ...     sandbox = client.sandbox.create(template="python-data-science")
    ...
    ...     # Wait for it to be ready
    ...     sandbox = client.sandbox.wait_for_ready(sandbox.id)
    ...
    ...     # Execute a command
    ...     result = client.sandbox.execute(
    ...         sandbox.id,
    ...         command=["python", "-c", "print('Hello from sandbox!')"],
    ...     )
    ...     print(result.stdout)
    ...
    ...     # Clean up
    ...     client.sandbox.delete(sandbox.id)
"""

from ._version import __version__
from .async_client import AsyncClient
from .client import Client
from .exceptions import (
    APIError,
    BadRequestError,
    ConflictError,
    InternalServerError,
    LiteBoxdError,
    NotFoundError,
    PrepullFailedError,
    SandboxFailedError,
    TimeoutError,
    UnauthorizedError,
)
from .models import (
    CreateSandboxRequest,
    CreateTemplateRequest,
    ExecAction,
    ExecResult,
    FileSpec,
    ImportAction,
    ImportResult,
    ImportResultItem,
    ImportStrategy,
    LogsResult,
    NetworkSpec,
    PrepullProgress,
    PrepullStatus,
    PrepullTask,
    ProbeSpec,
    ResourceSpec,
    RollbackResult,
    Sandbox,
    SandboxOverrides,
    SandboxStatus,
    Template,
    TemplateListResult,
    TemplateSpec,
    TemplateVersion,
    UpdateTemplateRequest,
    VersionListResult,
)

__all__ = [
    # Version
    "__version__",
    # Client
    "Client",
    "AsyncClient",
    # Exceptions
    "LiteBoxdError",
    "APIError",
    "NotFoundError",
    "ConflictError",
    "BadRequestError",
    "UnauthorizedError",
    "InternalServerError",
    "TimeoutError",
    "SandboxFailedError",
    "PrepullFailedError",
    # Sandbox models
    "Sandbox",
    "SandboxStatus",
    "SandboxOverrides",
    "CreateSandboxRequest",
    "ExecResult",
    "LogsResult",
    # Template models
    "Template",
    "TemplateSpec",
    "TemplateVersion",
    "ResourceSpec",
    "FileSpec",
    "ExecAction",
    "ProbeSpec",
    "NetworkSpec",
    "CreateTemplateRequest",
    "UpdateTemplateRequest",
    "RollbackResult",
    "TemplateListResult",
    "VersionListResult",
    # Prepull models
    "PrepullStatus",
    "PrepullProgress",
    "PrepullTask",
    # Import/Export models
    "ImportStrategy",
    "ImportAction",
    "ImportResultItem",
    "ImportResult",
]
