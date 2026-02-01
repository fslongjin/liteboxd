"""Tests for import/export service."""

import pytest
from pytest_httpx import HTTPXMock

from liteboxd import Client, ImportAction, ImportStrategy


class TestImportExportService:
    """Tests for ImportExportService."""

    def test_import_templates(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test importing templates."""
        import_result = {
            "total": 2,
            "created": 1,
            "updated": 1,
            "skipped": 0,
            "failed": 0,
            "results": [
                {"name": "template-1", "action": "created", "version": 1},
                {"name": "template-2", "action": "updated", "version": 2},
            ],
        }
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/templates/import",
            json=import_result,
        )

        yaml_content = b"""
apiVersion: liteboxd/v1
kind: SandboxTemplate
metadata:
  name: template-1
spec:
  image: python:3.11-slim
"""

        with Client(base_url) as client:
            result = client.import_export.import_templates(
                yaml_content,
                strategy=ImportStrategy.CREATE_OR_UPDATE,
                auto_prepull=True,
            )
            assert result.total == 2
            assert result.created == 1
            assert result.updated == 1
            assert len(result.results) == 2
            assert result.results[0].action == ImportAction.CREATED

    def test_import_templates_with_string(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test importing templates with string content."""
        import_result = {
            "total": 1,
            "created": 1,
            "updated": 0,
            "skipped": 0,
            "failed": 0,
            "results": [{"name": "template-1", "action": "created", "version": 1}],
        }
        httpx_mock.add_response(
            method="POST",
            url=f"{base_url}/templates/import",
            json=import_result,
        )

        yaml_content = """
apiVersion: liteboxd/v1
kind: SandboxTemplate
metadata:
  name: template-1
spec:
  image: python:3.11-slim
"""

        with Client(base_url) as client:
            result = client.import_export.import_templates(yaml_content)
            assert result.created == 1

    def test_export_all_templates(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test exporting all templates."""
        yaml_content = b"""
apiVersion: liteboxd/v1
kind: SandboxTemplateList
items:
  - metadata:
      name: template-1
    spec:
      image: python:3.11-slim
"""
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/templates/export",
            content=yaml_content,
        )

        with Client(base_url) as client:
            content = client.import_export.export_all()
            assert b"SandboxTemplateList" in content

    def test_export_templates_with_filters(
        self,
        base_url: str,
        httpx_mock: HTTPXMock,
    ) -> None:
        """Test exporting templates with filters."""
        yaml_content = b"apiVersion: liteboxd/v1\n"
        httpx_mock.add_response(
            method="GET",
            url=f"{base_url}/templates/export?tag=python&names=tpl-1%2Ctpl-2",
            content=yaml_content,
        )

        with Client(base_url) as client:
            content = client.import_export.export_all(
                tag="python",
                names=["tpl-1", "tpl-2"],
            )
            assert content == yaml_content
