"""Tests for template service."""

import pytest
from pytest_httpx import HTTPXMock

from liteboxd import (
    Client,
    ConflictError,
    CreateTemplateRequest,
    NotFoundError,
    ResourceSpec,
    TemplateSpec,
    UpdateTemplateRequest,
)


class TestTemplateService:
    """Tests for TemplateService."""

    def test_create_template(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        template_data: dict,
    ) -> None:
        """Test creating a template."""
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/templates",
            json=template_data,
            status_code=201,
        )

        with Client(base_url) as client:
            request = CreateTemplateRequest(
                name="python-data-science",
                display_name="Python Data Science",
                description="Python environment with data science packages",
                tags=["python", "data-science"],
                spec=TemplateSpec(
                    image="python:3.11-slim",
                    resources=ResourceSpec(cpu="500m", memory="512Mi"),
                    ttl=3600,
                ),
            )
            template = client.template.create(request)
            assert template.name == "python-data-science"

    def test_create_template_conflict(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test creating a template that already exists."""
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/templates",
            json={"error": "template already exists"},
            status_code=409,
        )

        with Client(base_url) as client:
            request = CreateTemplateRequest(
                name="python-data-science",
                spec=TemplateSpec(image="python:3.11-slim"),
            )
            with pytest.raises(ConflictError):
                client.template.create(request)

    def test_list_templates(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        template_data: dict,
    ) -> None:
        """Test listing templates."""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/templates?page=1&pageSize=20",
            json={"items": [template_data], "total": 1, "page": 1, "pageSize": 20},
        )

        with Client(base_url) as client:
            result = client.template.list()
            assert len(result.items) == 1
            assert result.items[0].name == "python-data-science"

    def test_list_templates_with_filters(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        template_data: dict,
    ) -> None:
        """Test listing templates with filters."""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/templates?page=1&pageSize=10&tag=python&search=data",
            json={"items": [template_data], "total": 1, "page": 1, "pageSize": 10},
        )

        with Client(base_url) as client:
            result = client.template.list(
                tag="python",
                search="data",
                page=1,
                page_size=10,
            )
            assert len(result.items) == 1

    def test_get_template(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        template_data: dict,
    ) -> None:
        """Test getting a template."""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/templates/python-data-science",
            json=template_data,
        )

        with Client(base_url) as client:
            template = client.template.get("python-data-science")
            assert template.name == "python-data-science"
            assert template.latest_version == 1

    def test_get_template_not_found(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test getting a non-existent template."""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/templates/nonexistent",
            json={"error": "template not found"},
            status_code=404,
        )

        with Client(base_url) as client:
            with pytest.raises(NotFoundError):
                client.template.get("nonexistent")

    def test_update_template(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
        template_data: dict,
    ) -> None:
        """Test updating a template."""
        updated_data = {**template_data, "latestVersion": 2}
        httpx_mock.add_response(
            method="PUT",
            url=f"{base_url}/templates/python-data-science",
            json=updated_data,
        )

        with Client(base_url) as client:
            request = UpdateTemplateRequest(
                description="Updated description",
                spec=TemplateSpec(image="python:3.12-slim"),
                changelog="Updated to Python 3.12",
            )
            template = client.template.update("python-data-science", request)
            assert template.latest_version == 2

    def test_delete_template(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test deleting a template."""
        httpx_mock.add_response(
            method="DELETE",
            url=f"{base_url}/templates/python-data-science",
            status_code=204,
        )

        with Client(base_url) as client:
            client.template.delete("python-data-science")

    def test_list_versions(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test listing template versions."""
        version_data = {
            "items": [
                {
                    "id": "ver-1",
                    "templateId": "tpl-123",
                    "version": 1,
                    "spec": {"image": "python:3.11-slim", "resources": {"cpu": "500m", "memory": "512Mi"}, "ttl": 3600},
                    "createdAt": "2024-01-01T00:00:00Z",
                }
            ],
            "total": 1,
        }
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/templates/python-data-science/versions",
            json=version_data,
        )

        with Client(base_url) as client:
            result = client.template.list_versions("python-data-science")
            assert len(result.items) == 1
            assert result.items[0].version == 1

    def test_rollback_template(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test rolling back a template."""
        rollback_data = {
            "id": "tpl-123",
            "name": "python-data-science",
            "latestVersion": 3,
            "rolledBackFrom": 2,
            "rolledBackTo": 1,
            "updatedAt": "2024-01-01T00:00:00Z",
        }
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/templates/python-data-science/rollback",
            json=rollback_data,
        )

        with Client(base_url) as client:
            result = client.template.rollback(
                "python-data-science",
                target_version=1,
                changelog="Rollback to v1",
            )
            assert result.latest_version == 3
            assert result.rolled_back_to == 1

    def test_export_yaml(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test exporting a template as YAML."""
        yaml_content = b"apiVersion: liteboxd/v1\nkind: SandboxTemplate\n"
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/templates/python-data-science/export",
            content=yaml_content,
        )

        with Client(base_url) as client:
            content = client.template.export_yaml("python-data-science")
            assert content == yaml_content
