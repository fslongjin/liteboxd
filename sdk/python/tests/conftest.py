"""Pytest configuration and fixtures."""

import pytest


@pytest.fixture
def base_url() -> str:
    """Return the base URL for testing."""
    return "http://localhost:8080/api/v1"


@pytest.fixture
def sandbox_data() -> dict:
    """Return sample sandbox data."""
    return {
        "id": "sandbox-123",
        "image": "python:3.11-slim",
        "cpu": "500m",
        "memory": "512Mi",
        "ttl": 3600,
        "status": "running",
        "template": "python-data-science",
        "templateVersion": 1,
        "created_at": "2024-01-01T00:00:00Z",
        "expires_at": "2024-01-01T01:00:00Z",
    }


@pytest.fixture
def template_data() -> dict:
    """Return sample template data."""
    return {
        "id": "tpl-123",
        "name": "python-data-science",
        "displayName": "Python Data Science",
        "description": "Python environment with data science packages",
        "tags": ["python", "data-science"],
        "author": "admin",
        "isPublic": True,
        "latestVersion": 1,
        "createdAt": "2024-01-01T00:00:00Z",
        "updatedAt": "2024-01-01T00:00:00Z",
        "spec": {
            "image": "python:3.11-slim",
            "resources": {"cpu": "500m", "memory": "512Mi"},
            "ttl": 3600,
        },
    }


@pytest.fixture
def exec_result_data() -> dict:
    """Return sample exec result data."""
    return {
        "exit_code": 0,
        "stdout": "Hello, World!\n",
        "stderr": "",
    }
