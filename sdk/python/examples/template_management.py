#!/usr/bin/env python3
"""Template management example for LiteBoxd Python SDK."""

from liteboxd import (
    Client,
    ConflictError,
    CreateTemplateRequest,
    ImportStrategy,
    NotFoundError,
    ResourceSpec,
    TemplateSpec,
    UpdateTemplateRequest,
)


def main() -> None:
    client = Client("http://localhost:8080/api/v1")

    try:
        # Create a template
        print("Creating template...")
        try:
            template = client.template.create(
                CreateTemplateRequest(
                    name="example-python",
                    display_name="Example Python Environment",
                    description="A Python environment for examples",
                    tags=["python", "example"],
                    spec=TemplateSpec(
                        image="python:3.11-slim",
                        resources=ResourceSpec(cpu="500m", memory="512Mi"),
                        ttl=3600,
                        startup_script="pip install requests",
                        startup_timeout=120,
                    ),
                    auto_prepull=True,
                )
            )
            print(f"Created template: {template.name} v{template.latest_version}")
        except ConflictError:
            print("Template already exists, getting it...")
            template = client.template.get("example-python")

        # List templates
        print("\nListing templates...")
        result = client.template.list(tag="python")
        for t in result.items:
            print(f"  - {t.name}: {t.description}")

        # Update template
        print("\nUpdating template...")
        template = client.template.update(
            "example-python",
            UpdateTemplateRequest(
                description="Updated Python environment",
                spec=TemplateSpec(
                    image="python:3.12-slim",
                    resources=ResourceSpec(cpu="1000m", memory="1Gi"),
                    ttl=7200,
                ),
                changelog="Updated to Python 3.12",
            ),
        )
        print(f"Updated template to v{template.latest_version}")

        # List versions
        print("\nListing versions...")
        versions = client.template.list_versions("example-python")
        for v in versions.items:
            print(f"  - Version {v.version}: {v.changelog or 'Initial version'}")

        # Export template
        print("\nExporting template...")
        yaml_content = client.template.export_yaml("example-python")
        print(f"Exported YAML ({len(yaml_content)} bytes):")
        print(yaml_content.decode()[:500] + "...")

        # Rollback (if we have multiple versions)
        if template.latest_version > 1:
            print("\nRolling back to version 1...")
            result = client.template.rollback(
                "example-python",
                target_version=1,
                changelog="Rolled back for testing",
            )
            print(f"Rolled back from v{result.rolled_back_from} to v{result.rolled_back_to}")
            print(f"New version: v{result.latest_version}")

        # Export all templates
        print("\nExporting all templates...")
        all_yaml = client.import_export.export_all(tag="python")
        print(f"Exported all templates ({len(all_yaml)} bytes)")

        # Import templates (example)
        print("\nImport example (dry run - using same content)...")
        import_result = client.import_export.import_templates(
            yaml_content,
            strategy=ImportStrategy.UPDATE_ONLY,
        )
        print(f"Import result: {import_result.updated} updated, {import_result.skipped} skipped")

    finally:
        client.close()


if __name__ == "__main__":
    main()
