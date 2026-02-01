# LiteBoxd Python SDK 设计文档

## 1. 概述

本文档描述 LiteBoxd Python SDK 的设计和实现计划。

### 1.1 目标

- 提供类型安全、符合 Python 惯例的 SDK
- 支持同步和异步（async/await）两种使用模式
- 完整覆盖所有 LiteBoxd API 功能
- 提供良好的开发体验（类型提示、文档字符串、自动补全）
- 与 Go SDK 保持功能对等和 API 设计一致性

### 1.2 非目标

- WebSocket 交互式终端支持（第一阶段不实现）
- 命令行工具（已有独立的 CLI）
- 与其他云服务的集成

---

## 2. 技术选型

### 2.1 核心依赖

| 依赖 | 用途 | 版本要求 |
|------|------|----------|
| `httpx` | HTTP 客户端（支持同步和异步） | >= 0.24.0 |
| `pydantic` | 数据验证和序列化 | >= 2.0.0 |
| `typing-extensions` | 类型提示扩展 | >= 4.0.0 |

### 2.2 开发依赖

| 依赖 | 用途 |
|------|------|
| `pytest` | 测试框架 |
| `pytest-asyncio` | 异步测试支持 |
| `pytest-httpx` | HTTP mock |
| `mypy` | 类型检查 |
| `ruff` | 代码检查和格式化 |

### 2.3 Python 版本

- 最低支持版本：Python 3.10+
- 推荐版本：Python 3.11+

---

## 3. 项目结构

```
sdk/python/
├── pyproject.toml           # 项目配置和依赖
├── README.md                # SDK 文档
├── LICENSE
├── src/
│   └── liteboxd/
│       ├── __init__.py      # 公开 API 导出
│       ├── py.typed         # PEP 561 类型标记
│       ├── client.py        # 主客户端类
│       ├── _base.py         # 基础 HTTP 客户端
│       ├── _async_base.py   # 异步基础客户端
│       ├── models/
│       │   ├── __init__.py
│       │   ├── sandbox.py   # 沙箱相关模型
│       │   ├── template.py  # 模板相关模型
│       │   ├── prepull.py   # 预拉取相关模型
│       │   └── common.py    # 公共模型
│       ├── services/
│       │   ├── __init__.py
│       │   ├── sandbox.py   # 沙箱服务
│       │   ├── template.py  # 模板服务
│       │   ├── prepull.py   # 预拉取服务
│       │   └── import_export.py  # 导入/导出服务
│       ├── exceptions.py    # 异常定义
│       └── _version.py      # 版本信息
├── tests/
│   ├── __init__.py
│   ├── conftest.py          # pytest 配置和 fixtures
│   ├── test_client.py
│   ├── test_sandbox.py
│   ├── test_template.py
│   ├── test_prepull.py
│   └── test_import_export.py
└── examples/
    ├── quickstart.py
    ├── async_example.py
    ├── sandbox_lifecycle.py
    └── template_management.py
```

**包名**: `liteboxd`
**PyPI 发布名**: `liteboxd-sdk`

---

## 4. 核心客户端设计

### 4.1 同步客户端

```python
from liteboxd import Client

# 基础使用
client = Client("http://localhost:8080/api/v1")

# 带配置选项
client = Client(
    base_url="http://localhost:8080/api/v1",
    timeout=30.0,
    auth_token="your-token",
    headers={"X-Custom-Header": "value"},
)

# 作为上下文管理器
with Client("http://localhost:8080/api/v1") as client:
    sandbox = client.sandbox.create(template="python-data-science")
```

### 4.2 异步客户端

```python
from liteboxd import AsyncClient

async with AsyncClient("http://localhost:8080/api/v1") as client:
    sandbox = await client.sandbox.create(template="python-data-science")
```

### 4.3 客户端类定义

```python
from typing import Optional
from httpx import Client as HTTPClient, Timeout

class Client:
    """LiteBoxd API 同步客户端"""

    def __init__(
        self,
        base_url: str = "http://localhost:8080/api/v1",
        *,
        timeout: float | Timeout = 30.0,
        auth_token: Optional[str] = None,
        headers: Optional[dict[str, str]] = None,
        http_client: Optional[HTTPClient] = None,
    ) -> None:
        ...

    @property
    def sandbox(self) -> SandboxService:
        """沙箱操作服务"""
        ...

    @property
    def template(self) -> TemplateService:
        """模板操作服务"""
        ...

    @property
    def prepull(self) -> PrepullService:
        """镜像预拉取服务"""
        ...

    @property
    def import_export(self) -> ImportExportService:
        """模板导入/导出服务"""
        ...

    def close(self) -> None:
        """关闭 HTTP 客户端连接"""
        ...

    def __enter__(self) -> "Client":
        ...

    def __exit__(self, *args) -> None:
        ...
```

---

## 5. 服务客户端接口

### 5.1 SandboxService

```python
class SandboxService:
    """沙箱操作服务"""

    def create(
        self,
        template: str,
        *,
        template_version: Optional[int] = None,
        overrides: Optional[SandboxOverrides] = None,
    ) -> Sandbox:
        """
        从模板创建沙箱

        Args:
            template: 模板名称（必填）
            template_version: 模板版本（可选，默认使用最新版本）
            overrides: 覆盖配置（cpu, memory, ttl, env）

        Returns:
            创建的沙箱对象

        Raises:
            NotFoundError: 模板不存在
            BadRequestError: 请求参数无效
        """
        ...

    def list(self) -> list[Sandbox]:
        """获取所有沙箱列表"""
        ...

    def get(self, sandbox_id: str) -> Sandbox:
        """获取指定沙箱详情"""
        ...

    def delete(self, sandbox_id: str) -> None:
        """删除沙箱"""
        ...

    def execute(
        self,
        sandbox_id: str,
        command: list[str],
        *,
        timeout: int = 30,
    ) -> ExecResult:
        """
        在沙箱中执行命令

        Args:
            sandbox_id: 沙箱 ID
            command: 命令参数列表，例如 ["python", "-c", "print('hello')"]
            timeout: 执行超时时间（秒）

        Returns:
            命令执行结果（exit_code, stdout, stderr）
        """
        ...

    def get_logs(self, sandbox_id: str) -> LogsResult:
        """获取沙箱日志和事件"""
        ...

    def upload_file(
        self,
        sandbox_id: str,
        path: str,
        content: bytes,
        *,
        content_type: str = "application/octet-stream",
    ) -> None:
        """上传文件到沙箱"""
        ...

    def download_file(self, sandbox_id: str, path: str) -> bytes:
        """从沙箱下载文件"""
        ...

    def wait_for_ready(
        self,
        sandbox_id: str,
        *,
        poll_interval: float = 2.0,
        timeout: float = 300.0,
    ) -> Sandbox:
        """
        等待沙箱就绪

        Args:
            sandbox_id: 沙箱 ID
            poll_interval: 轮询间隔（秒）
            timeout: 最大等待时间（秒）

        Returns:
            就绪状态的沙箱对象

        Raises:
            TimeoutError: 等待超时
            SandboxFailedError: 沙箱启动失败
        """
        ...
```

### 5.2 TemplateService

```python
class TemplateService:
    """模板操作服务"""

    def create(self, request: CreateTemplateRequest) -> Template:
        """创建新模板"""
        ...

    def list(
        self,
        *,
        tag: Optional[str] = None,
        search: Optional[str] = None,
        page: int = 1,
        page_size: int = 20,
    ) -> TemplateListResult:
        """获取模板列表（支持分页和过滤）"""
        ...

    def get(self, name: str) -> Template:
        """获取指定模板详情"""
        ...

    def update(self, name: str, request: UpdateTemplateRequest) -> Template:
        """更新模板（创建新版本）"""
        ...

    def delete(self, name: str) -> None:
        """删除模板"""
        ...

    def list_versions(self, name: str) -> VersionListResult:
        """获取模板的所有版本"""
        ...

    def get_version(self, name: str, version: int) -> TemplateVersion:
        """获取模板的指定版本"""
        ...

    def rollback(
        self,
        name: str,
        target_version: int,
        *,
        changelog: Optional[str] = None,
    ) -> RollbackResult:
        """回滚模板到指定版本"""
        ...

    def export_yaml(
        self,
        name: str,
        *,
        version: Optional[int] = None,
    ) -> bytes:
        """导出模板为 YAML 格式"""
        ...
```

### 5.3 PrepullService

```python
class PrepullService:
    """镜像预拉取服务"""

    def create(
        self,
        image: str,
        *,
        timeout: int = 600,
    ) -> PrepullTask:
        """创建镜像预拉取任务"""
        ...

    def create_for_template(self, template_name: str) -> PrepullTask:
        """为模板创建预拉取任务"""
        ...

    def list(
        self,
        *,
        image: Optional[str] = None,
        status: Optional[PrepullStatus] = None,
    ) -> list[PrepullTask]:
        """获取预拉取任务列表"""
        ...

    def get(self, task_id: str) -> PrepullTask:
        """获取指定预拉取任务"""
        ...

    def delete(self, task_id: str) -> None:
        """删除预拉取任务"""
        ...

    def wait_for_completion(
        self,
        task_id: str,
        *,
        poll_interval: float = 5.0,
        timeout: float = 1800.0,
    ) -> PrepullTask:
        """等待预拉取任务完成"""
        ...
```

### 5.4 ImportExportService

```python
class ImportExportService:
    """模板导入/导出服务"""

    def import_templates(
        self,
        yaml_content: bytes | str,
        *,
        strategy: ImportStrategy = ImportStrategy.CREATE_OR_UPDATE,
        auto_prepull: bool = False,
    ) -> ImportResult:
        """
        从 YAML 导入模板

        Args:
            yaml_content: YAML 内容
            strategy: 导入策略
            auto_prepull: 是否自动预拉取镜像
        """
        ...

    def export_all(
        self,
        *,
        tag: Optional[str] = None,
        names: Optional[list[str]] = None,
    ) -> bytes:
        """导出所有模板为 YAML"""
        ...
```

---

## 6. 数据模型定义

### 6.1 沙箱模型

```python
from datetime import datetime
from enum import Enum
from typing import Optional
from pydantic import BaseModel, Field

class SandboxStatus(str, Enum):
    PENDING = "pending"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    TERMINATING = "terminating"
    UNKNOWN = "unknown"

class Sandbox(BaseModel):
    """沙箱对象"""
    id: str
    image: str
    cpu: str
    memory: str
    ttl: int
    env: Optional[dict[str, str]] = None
    status: SandboxStatus
    template: Optional[str] = None
    template_version: Optional[int] = Field(None, alias="templateVersion")
    created_at: datetime = Field(alias="created_at")
    expires_at: datetime = Field(alias="expires_at")
    access_token: Optional[str] = Field(None, alias="accessToken")
    access_url: Optional[str] = Field(None, alias="accessUrl")

    model_config = {"populate_by_name": True}

class SandboxOverrides(BaseModel):
    """沙箱配置覆盖"""
    cpu: Optional[str] = None
    memory: Optional[str] = None
    ttl: Optional[int] = None
    env: Optional[dict[str, str]] = None

class ExecResult(BaseModel):
    """命令执行结果"""
    exit_code: int = Field(alias="exit_code")
    stdout: str
    stderr: str

    model_config = {"populate_by_name": True}

class LogsResult(BaseModel):
    """日志查询结果"""
    logs: str
    events: list[str]
```

### 6.2 模板模型

```python
class ResourceSpec(BaseModel):
    """资源限制规格"""
    cpu: str = "500m"
    memory: str = "512Mi"

class FileSpec(BaseModel):
    """文件规格"""
    source: Optional[str] = None
    destination: str
    content: Optional[str] = None

class ExecAction(BaseModel):
    """执行动作"""
    command: list[str]

class ProbeSpec(BaseModel):
    """探针规格"""
    exec: ExecAction
    initial_delay_seconds: int = Field(0, alias="initialDelaySeconds")
    period_seconds: int = Field(10, alias="periodSeconds")
    failure_threshold: int = Field(3, alias="failureThreshold")

    model_config = {"populate_by_name": True}

class NetworkSpec(BaseModel):
    """网络配置"""
    allow_internet_access: bool = Field(False, alias="allowInternetAccess")
    allowed_domains: Optional[list[str]] = Field(None, alias="allowedDomains")

    model_config = {"populate_by_name": True}

class TemplateSpec(BaseModel):
    """模板规格"""
    image: str
    command: Optional[list[str]] = None
    args: Optional[list[str]] = None
    resources: ResourceSpec = Field(default_factory=ResourceSpec)
    ttl: int = 3600
    env: Optional[dict[str, str]] = None
    startup_script: Optional[str] = Field(None, alias="startupScript")
    startup_timeout: int = Field(300, alias="startupTimeout")
    files: Optional[list[FileSpec]] = None
    readiness_probe: Optional[ProbeSpec] = Field(None, alias="readinessProbe")
    network: Optional[NetworkSpec] = None

    model_config = {"populate_by_name": True}

class Template(BaseModel):
    """模板对象"""
    id: str
    name: str
    display_name: Optional[str] = Field(None, alias="displayName")
    description: Optional[str] = None
    tags: Optional[list[str]] = None
    author: Optional[str] = None
    is_public: bool = Field(True, alias="isPublic")
    latest_version: int = Field(alias="latestVersion")
    created_at: datetime = Field(alias="createdAt")
    updated_at: datetime = Field(alias="updatedAt")
    spec: Optional[TemplateSpec] = None

    model_config = {"populate_by_name": True}

class TemplateVersion(BaseModel):
    """模板版本"""
    id: str
    template_id: str = Field(alias="templateId")
    version: int
    spec: TemplateSpec
    changelog: Optional[str] = None
    created_by: Optional[str] = Field(None, alias="createdBy")
    created_at: datetime = Field(alias="createdAt")

    model_config = {"populate_by_name": True}

class CreateTemplateRequest(BaseModel):
    """创建模板请求"""
    name: str
    display_name: Optional[str] = Field(None, alias="displayName")
    description: Optional[str] = None
    tags: Optional[list[str]] = None
    is_public: Optional[bool] = Field(None, alias="isPublic")
    spec: TemplateSpec
    auto_prepull: bool = Field(False, alias="autoPrepull")

    model_config = {"populate_by_name": True}

class UpdateTemplateRequest(BaseModel):
    """更新模板请求"""
    display_name: Optional[str] = Field(None, alias="displayName")
    description: Optional[str] = None
    tags: Optional[list[str]] = None
    is_public: Optional[bool] = Field(None, alias="isPublic")
    spec: TemplateSpec
    changelog: Optional[str] = None

    model_config = {"populate_by_name": True}
```

### 6.3 预拉取模型

```python
class PrepullStatus(str, Enum):
    PENDING = "pending"
    PULLING = "pulling"
    COMPLETED = "completed"
    FAILED = "failed"

class PrepullProgress(BaseModel):
    """预拉取进度"""
    ready: int
    total: int

class PrepullTask(BaseModel):
    """预拉取任务"""
    id: str
    image: str
    status: PrepullStatus
    progress: Optional[PrepullProgress] = None
    template: Optional[str] = None
    error: Optional[str] = None
    started_at: datetime = Field(alias="startedAt")
    completed_at: Optional[datetime] = Field(None, alias="completedAt")

    model_config = {"populate_by_name": True}
```

### 6.4 导入/导出模型

```python
class ImportStrategy(str, Enum):
    CREATE_ONLY = "create-only"
    UPDATE_ONLY = "update-only"
    CREATE_OR_UPDATE = "create-or-update"

class ImportAction(str, Enum):
    CREATED = "created"
    UPDATED = "updated"
    SKIPPED = "skipped"
    FAILED = "failed"

class ImportResultItem(BaseModel):
    """单个模板导入结果"""
    name: str
    action: ImportAction
    version: Optional[int] = None
    error: Optional[str] = None

class ImportResult(BaseModel):
    """导入结果"""
    total: int
    created: int
    updated: int
    skipped: int
    failed: int
    results: list[ImportResultItem]
    prepull_started: Optional[list[str]] = Field(None, alias="prepullStarted")

    model_config = {"populate_by_name": True}
```

---

## 7. 异常处理

```python
class LiteBoxdError(Exception):
    """LiteBoxd SDK 基础异常"""
    pass

class APIError(LiteBoxdError):
    """API 错误响应"""

    def __init__(self, status_code: int, message: str):
        self.status_code = status_code
        self.message = message
        super().__init__(f"[{status_code}] {message}")

class NotFoundError(APIError):
    """资源不存在 (404)"""

    def __init__(self, message: str = "Resource not found"):
        super().__init__(404, message)

class ConflictError(APIError):
    """资源冲突 (409)"""

    def __init__(self, message: str = "Resource already exists"):
        super().__init__(409, message)

class BadRequestError(APIError):
    """请求无效 (400)"""

    def __init__(self, message: str = "Invalid request"):
        super().__init__(400, message)

class UnauthorizedError(APIError):
    """未授权 (401)"""

    def __init__(self, message: str = "Unauthorized"):
        super().__init__(401, message)

class InternalServerError(APIError):
    """服务器内部错误 (500)"""

    def __init__(self, message: str = "Internal server error"):
        super().__init__(500, message)

class TimeoutError(LiteBoxdError):
    """操作超时"""
    pass

class SandboxFailedError(LiteBoxdError):
    """沙箱启动失败"""

    def __init__(self, sandbox_id: str, message: str = "Sandbox failed to start"):
        self.sandbox_id = sandbox_id
        super().__init__(f"Sandbox {sandbox_id}: {message}")

class PrepullFailedError(LiteBoxdError):
    """预拉取任务失败"""

    def __init__(self, task_id: str, error: str):
        self.task_id = task_id
        self.error = error
        super().__init__(f"Prepull task {task_id} failed: {error}")
```

---

## 8. 实现计划

### 阶段 1：SDK 基础架构（2-3 天）

| 任务 | 描述 | 优先级 |
|------|------|--------|
| 1.1 | 初始化项目结构，配置 pyproject.toml | P0 |
| 1.2 | 实现基础 HTTP 客户端（同步） | P0 |
| 1.3 | 实现异常类型和错误处理 | P0 |
| 1.4 | 实现主客户端类 Client | P0 |
| 1.5 | 添加类型提示和 py.typed 标记 | P0 |
| 1.6 | 配置 pytest 和基础测试 | P0 |

### 阶段 2：沙箱服务（2-3 天）

| 任务 | 描述 | 优先级 |
|------|------|--------|
| 2.1 | 定义沙箱相关 Pydantic 模型 | P0 |
| 2.2 | 实现 SandboxService 基础 CRUD | P0 |
| 2.3 | 实现命令执行 (execute) | P0 |
| 2.4 | 实现日志获取 (get_logs) | P0 |
| 2.5 | 实现文件上传/下载 | P0 |
| 2.6 | 实现 wait_for_ready 轮询等待 | P0 |
| 2.7 | 添加沙箱服务单元测试 | P0 |

### 阶段 3：模板服务（2 天）

| 任务 | 描述 | 优先级 |
|------|------|--------|
| 3.1 | 定义模板相关 Pydantic 模型 | P0 |
| 3.2 | 实现 TemplateService CRUD | P0 |
| 3.3 | 实现版本管理（list_versions, get_version, rollback） | P0 |
| 3.4 | 实现 YAML 导出 | P0 |
| 3.5 | 添加模板服务单元测试 | P0 |

### 阶段 4：预拉取和导入/导出服务（1-2 天）

| 任务 | 描述 | 优先级 |
|------|------|--------|
| 4.1 | 定义预拉取相关模型 | P0 |
| 4.2 | 实现 PrepullService | P0 |
| 4.3 | 实现 wait_for_completion | P0 |
| 4.4 | 实现 ImportExportService | P0 |
| 4.5 | 添加相关单元测试 | P0 |

### 阶段 5：异步客户端（2 天）

| 任务 | 描述 | 优先级 |
|------|------|--------|
| 5.1 | 实现异步基础 HTTP 客户端 | P1 |
| 5.2 | 实现 AsyncClient | P1 |
| 5.3 | 实现所有服务的异步版本 | P1 |
| 5.4 | 添加异步测试 | P1 |

### 阶段 6：集成测试和文档（2 天）

| 任务 | 描述 | 优先级 |
|------|------|--------|
| 6.1 | 编写集成测试（需要运行后端） | P1 |
| 6.2 | 编写 SDK README | P0 |
| 6.3 | 编写 API 参考文档 | P1 |
| 6.4 | 编写快速入门指南 | P0 |
| 6.5 | 创建示例代码 | P0 |

### 阶段 7：发布准备（1 天）

| 任务 | 描述 | 优先级 |
|------|------|--------|
| 7.1 | 配置 CI/CD（GitHub Actions） | P1 |
| 7.2 | 配置 PyPI 发布 | P1 |
| 7.3 | 编写 CHANGELOG | P0 |
| 7.4 | 代码审查和最终测试 | P0 |

---

## 9. 使用示例

### 9.1 快速入门

```python
from liteboxd import Client

# 创建客户端
client = Client("http://localhost:8080/api/v1")

# 从模板创建沙箱
sandbox = client.sandbox.create(
    template="python-data-science",
    overrides={"ttl": 7200, "env": {"DEBUG": "true"}},
)
print(f"Created sandbox: {sandbox.id}")

# 等待沙箱就绪
sandbox = client.sandbox.wait_for_ready(sandbox.id)
print(f"Sandbox status: {sandbox.status}")

# 执行命令
result = client.sandbox.execute(
    sandbox.id,
    command=["python", "-c", "print('Hello from sandbox!')"],
)
print(f"Output: {result.stdout}")

# 清理
client.sandbox.delete(sandbox.id)
client.close()
```

### 9.2 异步使用

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
            command=["python", "-c", "print('Hello!')"],
        )
        print(result.stdout)

        # 清理
        await client.sandbox.delete(sandbox.id)

asyncio.run(main())
```

### 9.3 模板管理

```python
from liteboxd import Client
from liteboxd.models import CreateTemplateRequest, TemplateSpec, ResourceSpec

client = Client("http://localhost:8080/api/v1")

# 创建模板
template = client.template.create(
    CreateTemplateRequest(
        name="my-python-template",
        display_name="My Python Template",
        description="Custom Python environment",
        tags=["python", "custom"],
        spec=TemplateSpec(
            image="python:3.11-slim",
            resources=ResourceSpec(cpu="1000m", memory="1Gi"),
            ttl=7200,
            startup_script="pip install numpy pandas",
        ),
        auto_prepull=True,
    )
)
print(f"Created template: {template.name} v{template.latest_version}")

# 列出模板
templates = client.template.list(tag="python")
for t in templates.items:
    print(f"- {t.name}: {t.description}")

# 导出模板
yaml_content = client.template.export_yaml("my-python-template")
print(yaml_content.decode())

client.close()
```

---

## 10. 测试策略

### 10.1 单元测试

- 使用 `pytest-httpx` mock HTTP 请求
- 测试覆盖率目标：>= 80%
- 测试所有公开 API 方法
- 测试错误处理路径

### 10.2 集成测试

- 需要运行 LiteBoxd 后端服务
- 使用 pytest markers 区分集成测试
- 测试完整的工作流程

### 10.3 类型检查

- 使用 mypy 进行静态类型检查
- 配置严格模式

---

## 11. 版本策略

- 遵循语义化版本（SemVer）
- 初始版本：`0.1.0`
- API 稳定后发布 `1.0.0`

---

## 12. 与 Go SDK 的差异

| 功能 | Go SDK | Python SDK |
|------|--------|------------|
| HTTP 客户端 | net/http | httpx |
| 类型系统 | Go types | Pydantic models |
| 异步支持 | goroutine | async/await |
| 错误处理 | error interface | 异常类 |
| 上下文传递 | context.Context | 方法参数 |
| 超时处理 | context.WithTimeout | httpx Timeout |

---

## 13. 成功标准

- [ ] 完整覆盖所有 API 端点
- [ ] 测试覆盖率 >= 80%
- [ ] 类型检查通过（mypy strict）
- [ ] 文档完整（README、API 参考、示例）
- [ ] 可通过 pip 安装使用
- [ ] 支持 Python 3.10+
