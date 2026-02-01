# LiteBoxd Python SDK 快速入门

本指南帮助你快速上手 LiteBoxd Python SDK。

## 安装

```bash
pip install liteboxd-sdk
```

或使用 Poetry:

```bash
poetry add liteboxd-sdk
```

## 基础使用

### 1. 创建客户端

```python
from liteboxd import Client

# 连接到本地开发服务器
client = Client("http://localhost:8080/api/v1")

# 或带认证令牌
client = Client(
    "http://localhost:8080/api/v1",
    auth_token="your-api-token",
)
```

### 2. 创建沙箱

所有沙箱都必须从模板创建：

```python
# 使用默认配置创建沙箱
sandbox = client.sandbox.create(template="python-data-science")

# 带配置覆盖
from liteboxd.models import SandboxOverrides

sandbox = client.sandbox.create(
    template="python-data-science",
    overrides=SandboxOverrides(
        cpu="1000m",
        memory="1Gi",
        ttl=7200,  # 2小时
        env={"DEBUG": "true"},
    ),
)

print(f"Sandbox ID: {sandbox.id}")
print(f"Status: {sandbox.status}")
```

### 3. 等待沙箱就绪

```python
# 等待沙箱进入 running 状态
sandbox = client.sandbox.wait_for_ready(sandbox.id)
print(f"Sandbox is ready: {sandbox.status}")
```

### 4. 执行命令

```python
# 执行 Python 代码
result = client.sandbox.execute(
    sandbox.id,
    command=["python", "-c", "print('Hello, LiteBoxd!')"],
    timeout=30,
)

print(f"Exit code: {result.exit_code}")
print(f"Stdout: {result.stdout}")
print(f"Stderr: {result.stderr}")
```

### 5. 文件操作

```python
# 上传文件
code = b"""
import numpy as np
print(np.array([1, 2, 3]) * 2)
"""
client.sandbox.upload_file(
    sandbox.id,
    path="/workspace/script.py",
    content=code,
)

# 执行上传的脚本
result = client.sandbox.execute(
    sandbox.id,
    command=["python", "/workspace/script.py"],
)
print(result.stdout)

# 下载文件
content = client.sandbox.download_file(sandbox.id, "/workspace/output.txt")
print(content.decode())
```

### 6. 获取日志

```python
logs = client.sandbox.get_logs(sandbox.id)
print(f"Container logs:\n{logs.logs}")
print(f"Events:\n{logs.events}")
```

### 7. 清理

```python
# 删除沙箱
client.sandbox.delete(sandbox.id)

# 关闭客户端
client.close()
```

## 使用上下文管理器

推荐使用上下文管理器自动管理资源：

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
# 客户端自动关闭
```

## 异步使用

SDK 提供异步客户端，适用于异步应用：

```python
import asyncio
from liteboxd import AsyncClient

async def main():
    async with AsyncClient("http://localhost:8080/api/v1") as client:
        # 创建沙箱
        sandbox = await client.sandbox.create(template="python-data-science")

        # 等待就绪
        sandbox = await client.sandbox.wait_for_ready(sandbox.id)

        # 执行命令
        result = await client.sandbox.execute(
            sandbox.id,
            command=["python", "-c", "print('Hello async!')"],
        )
        print(result.stdout)

        # 清理
        await client.sandbox.delete(sandbox.id)

asyncio.run(main())
```

## 模板管理

### 创建模板

```python
from liteboxd.models import (
    CreateTemplateRequest,
    TemplateSpec,
    ResourceSpec,
)

template = client.template.create(
    CreateTemplateRequest(
        name="my-python-env",
        display_name="My Python Environment",
        description="Custom Python environment with data science packages",
        tags=["python", "data-science"],
        spec=TemplateSpec(
            image="python:3.11-slim",
            resources=ResourceSpec(cpu="500m", memory="512Mi"),
            ttl=3600,
            startup_script="pip install numpy pandas matplotlib",
            startup_timeout=300,
        ),
        auto_prepull=True,
    )
)

print(f"Created template: {template.name} v{template.latest_version}")
```

### 列出和搜索模板

```python
# 列出所有模板
result = client.template.list()
for t in result.items:
    print(f"- {t.name}: {t.description}")

# 按标签过滤
result = client.template.list(tag="python")

# 搜索模板
result = client.template.list(search="data science")
```

### 导出和导入模板

```python
# 导出模板为 YAML
yaml_content = client.template.export_yaml("my-python-env")
with open("template.yaml", "wb") as f:
    f.write(yaml_content)

# 导入模板
from liteboxd.models import ImportStrategy

with open("template.yaml", "rb") as f:
    result = client.import_export.import_templates(
        f.read(),
        strategy=ImportStrategy.CREATE_OR_UPDATE,
        auto_prepull=True,
    )

print(f"Imported: {result.created} created, {result.updated} updated")
```

## 错误处理

```python
from liteboxd import Client
from liteboxd.exceptions import (
    NotFoundError,
    ConflictError,
    BadRequestError,
    TimeoutError,
    SandboxFailedError,
)

client = Client("http://localhost:8080/api/v1")

try:
    sandbox = client.sandbox.get("nonexistent-id")
except NotFoundError as e:
    print(f"Sandbox not found: {e.message}")

try:
    template = client.template.create(...)
except ConflictError as e:
    print(f"Template already exists: {e.message}")

try:
    sandbox = client.sandbox.wait_for_ready(
        sandbox_id,
        timeout=60.0,  # 60秒超时
    )
except TimeoutError:
    print("Timeout waiting for sandbox to be ready")
except SandboxFailedError as e:
    print(f"Sandbox failed to start: {e}")
```

## 下一步

- 查看 [API 参考](./api-reference.md) 了解完整的 API 文档
- 查看 [示例代码](../../../sdk/python/examples/) 了解更多使用场景
- 阅读 [设计文档](./design.md) 了解 SDK 架构
