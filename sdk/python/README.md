# LiteBoxd Python SDK

Python SDK for [LiteBoxd](https://github.com/fslongjin/liteboxd) - A lightweight K8s sandbox system.

## Installation

```bash
pip install liteboxd-sdk
```

Or install from source:

```bash
cd sdk/python
pip install -e .
```

## Quick Start

```python
from liteboxd import Client

# Create a client
client = Client("http://localhost:8080/api/v1")

# Create a sandbox from a template
sandbox = client.sandbox.create(template="python-data-science")

# Wait for sandbox to be ready
sandbox = client.sandbox.wait_for_ready(sandbox.id)

# Execute a command
result = client.sandbox.execute(
    sandbox.id,
    command=["python", "-c", "print('Hello from sandbox!')"],
)
print(result.stdout)

# Clean up
client.sandbox.delete(sandbox.id)
client.close()
```

## Using Context Manager

```python
from liteboxd import Client

with Client("http://localhost:8080/api/v1") as client:
    sandbox = client.sandbox.create(template="python-data-science")
    sandbox = client.sandbox.wait_for_ready(sandbox.id)

    result = client.sandbox.execute(
        sandbox.id,
        command=["python", "-c", "print('Hello!')"],
    )
    print(result.stdout)

    client.sandbox.delete(sandbox.id)
```

## Async Support

```python
import asyncio
from liteboxd import AsyncClient

async def main():
    async with AsyncClient("http://localhost:8080/api/v1") as client:
        sandbox = await client.sandbox.create(template="python-data-science")
        sandbox = await client.sandbox.wait_for_ready(sandbox.id)

        result = await client.sandbox.execute(
            sandbox.id,
            command=["python", "-c", "print('Hello async!')"],
        )
        print(result.stdout)

        await client.sandbox.delete(sandbox.id)

asyncio.run(main())
```

## Features

- **Sandbox Management**: Create, list, get, delete sandboxes
- **Command Execution**: Execute commands in sandboxes
- **File Operations**: Upload and download files
- **Template Management**: Create, update, and manage templates
- **Version Control**: Template versioning and rollback
- **Image Prepull**: Prepull images to K8s nodes
- **Import/Export**: Import and export templates as YAML
- **Async Support**: Full async/await support

## API Reference

### Client

```python
from liteboxd import Client

client = Client(
    base_url="http://localhost:8080/api/v1",
    timeout=30.0,
    auth_token="your-token",  # optional
)
```

### Sandbox Operations

```python
# Create sandbox
sandbox = client.sandbox.create(
    template="python-data-science",
    template_version=1,  # optional
    overrides=SandboxOverrides(
        cpu="1000m",
        memory="1Gi",
        ttl=7200,
        env={"DEBUG": "true"},
    ),
)

# List sandboxes
sandboxes = client.sandbox.list()

# Get sandbox
sandbox = client.sandbox.get(sandbox_id)

# Delete sandbox
client.sandbox.delete(sandbox_id)

# Execute command
result = client.sandbox.execute(sandbox_id, ["python", "-c", "print('hi')"])

# Get logs
logs = client.sandbox.get_logs(sandbox_id)

# Upload file
client.sandbox.upload_file(sandbox_id, "/path/to/file", content)

# Download file
content = client.sandbox.download_file(sandbox_id, "/path/to/file")

# Wait for ready
sandbox = client.sandbox.wait_for_ready(sandbox_id, timeout=300.0)
```

### Template Operations

```python
from liteboxd import CreateTemplateRequest, TemplateSpec, ResourceSpec

# Create template
template = client.template.create(
    CreateTemplateRequest(
        name="my-template",
        display_name="My Template",
        spec=TemplateSpec(
            image="python:3.11-slim",
            resources=ResourceSpec(cpu="500m", memory="512Mi"),
            ttl=3600,
        ),
    )
)

# List templates
result = client.template.list(tag="python", search="data")

# Get template
template = client.template.get("my-template")

# Update template
template = client.template.update("my-template", UpdateTemplateRequest(...))

# Delete template
client.template.delete("my-template")

# List versions
versions = client.template.list_versions("my-template")

# Rollback
result = client.template.rollback("my-template", target_version=1)

# Export as YAML
yaml_content = client.template.export_yaml("my-template")
```

### Prepull Operations

```python
# Create prepull task
task = client.prepull.create("python:3.11-slim")

# Create for template
task = client.prepull.create_for_template("my-template")

# List tasks
tasks = client.prepull.list(status=PrepullStatus.PULLING)

# Wait for completion
task = client.prepull.wait_for_completion(task_id, timeout=1800.0)
```

### Import/Export Operations

```python
from liteboxd import ImportStrategy

# Import templates
result = client.import_export.import_templates(
    yaml_content,
    strategy=ImportStrategy.CREATE_OR_UPDATE,
    auto_prepull=True,
)

# Export all templates
yaml_content = client.import_export.export_all(tag="python")
```

## Error Handling

```python
from liteboxd import (
    Client,
    NotFoundError,
    ConflictError,
    BadRequestError,
    TimeoutError,
    SandboxFailedError,
)

try:
    sandbox = client.sandbox.get("nonexistent")
except NotFoundError as e:
    print(f"Sandbox not found: {e.message}")

try:
    sandbox = client.sandbox.wait_for_ready(sandbox_id, timeout=60.0)
except TimeoutError:
    print("Timeout waiting for sandbox")
except SandboxFailedError as e:
    print(f"Sandbox failed: {e}")
```

## Development

### Install development dependencies

```bash
pip install -e ".[dev]"
```

### Run tests

```bash
pytest
```

### Type checking

```bash
mypy src/liteboxd
```

### Code formatting

```bash
ruff format src tests
ruff check src tests --fix
```

## License

MIT License
