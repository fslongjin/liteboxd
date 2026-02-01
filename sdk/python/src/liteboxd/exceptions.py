"""Exception classes for LiteBoxd SDK."""

from __future__ import annotations


class LiteBoxdError(Exception):
    """Base exception for LiteBoxd SDK."""

    pass


class APIError(LiteBoxdError):
    """API error response from the server."""

    def __init__(self, status_code: int, message: str) -> None:
        self.status_code = status_code
        self.message = message
        super().__init__(f"[{status_code}] {message}")


class NotFoundError(APIError):
    """Resource not found (HTTP 404)."""

    def __init__(self, message: str = "Resource not found") -> None:
        super().__init__(404, message)


class ConflictError(APIError):
    """Resource conflict (HTTP 409)."""

    def __init__(self, message: str = "Resource already exists") -> None:
        super().__init__(409, message)


class BadRequestError(APIError):
    """Invalid request (HTTP 400)."""

    def __init__(self, message: str = "Invalid request") -> None:
        super().__init__(400, message)


class UnauthorizedError(APIError):
    """Unauthorized (HTTP 401)."""

    def __init__(self, message: str = "Unauthorized") -> None:
        super().__init__(401, message)


class InternalServerError(APIError):
    """Internal server error (HTTP 500)."""

    def __init__(self, message: str = "Internal server error") -> None:
        super().__init__(500, message)


class TimeoutError(LiteBoxdError):
    """Operation timed out."""

    pass


class SandboxFailedError(LiteBoxdError):
    """Sandbox failed to start."""

    def __init__(self, sandbox_id: str, message: str = "Sandbox failed to start") -> None:
        self.sandbox_id = sandbox_id
        super().__init__(f"Sandbox {sandbox_id}: {message}")


class PrepullFailedError(LiteBoxdError):
    """Prepull task failed."""

    def __init__(self, task_id: str, error: str) -> None:
        self.task_id = task_id
        self.error = error
        super().__init__(f"Prepull task {task_id} failed: {error}")


def raise_for_status(status_code: int, message: str) -> None:
    """Raise appropriate exception based on HTTP status code."""
    if status_code == 400:
        raise BadRequestError(message)
    elif status_code == 401:
        raise UnauthorizedError(message)
    elif status_code == 404:
        raise NotFoundError(message)
    elif status_code == 409:
        raise ConflictError(message)
    elif status_code >= 500:
        raise InternalServerError(message)
    elif status_code >= 400:
        raise APIError(status_code, message)
