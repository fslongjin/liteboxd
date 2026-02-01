# LiteBoxd Python SDK API 参考

## 客户端

### Client

同步 API 客户端。

```python
class Client:
    def __init__(
        self,
        base_url: str = "http://localhost:8080/api/v1",
        *,
        timeout: float | Timeout = 30.0,
        auth_token: Optional[str] = None,
        headers: Optional[dict[str, str]] = None,
        http_client: Optional[HTTPClient] = None,
    ) -> None
```

**参数:**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `base_url` | `str` | `"http://localhost:8080/api/v1"` | API 基础 URL |
| `timeout` | `float \| Timeout` | `30.0` | 请求超时时间（秒） |
| `auth_token` | `Optional[str]` | `None` | 认证令牌 |
| `headers` | `Optional[dict[str, str]]` | `None` | 自定义 HTTP 头 |
| `http_client` | `Optional[HTTPClient]` | `None` | 自定义 httpx 客户端 |

**属性:**

| 属性 | 类型 | 说明 |
|------|------|------|
| `sandbox` | `SandboxService` | 沙箱操作服务 |
| `template` | `TemplateService` | 模板操作服务 |
| `prepull` | `PrepullService` | 镜像预拉取服务 |
| `import_export` | `ImportExportService` | 模板导入/导出服务 |

**方法:**

- `close() -> None` - 关闭 HTTP 客户端连接
- `__enter__() -> Client` - 上下文管理器入口
- `__exit__(*args) -> None` - 上下文管理器出口

---

### AsyncClient

异步 API 客户端，接口与 `Client` 相同，所有方法返回 `Awaitable`。

```python
async with AsyncClient("http://localhost:8080/api/v1") as client:
    sandbox = await client.sandbox.create(template="python-data-science")
```

---

## 沙箱服务 (SandboxService)

### create

从模板创建沙箱。

```python
def create(
    self,
    template: str,
    *,
    template_version: Optional[int] = None,
    overrides: Optional[SandboxOverrides] = None,
) -> Sandbox
```

**参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `template` | `str` | 模板名称（必填） |
| `template_version` | `Optional[int]` | 模板版本，默认使用最新版本 |
| `overrides` | `Optional[SandboxOverrides]` | 配置覆盖 |

**返回:** `Sandbox` - 创建的沙箱对象

**异常:**

- `NotFoundError` - 模板不存在
- `BadRequestError` - 请求参数无效

---

### list

获取所有沙箱列表。

```python
def list(self) -> list[Sandbox]
```

---

### get

获取指定沙箱详情。

```python
def get(self, sandbox_id: str) -> Sandbox
```

**异常:** `NotFoundError` - 沙箱不存在

---

### delete

删除沙箱。

```python
def delete(self, sandbox_id: str) -> None
```

---

### execute

在沙箱中执行命令。

```python
def execute(
    self,
    sandbox_id: str,
    command: list[str],
    *,
    timeout: int = 30,
) -> ExecResult
```

**参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `sandbox_id` | `str` | 沙箱 ID |
| `command` | `list[str]` | 命令参数列表 |
| `timeout` | `int` | 执行超时时间（秒） |

**示例:**

```python
result = client.sandbox.execute(
    sandbox.id,
    command=["python", "-c", "print('hello')"],
    timeout=30,
)
print(result.stdout)
```

---

### get_logs

获取沙箱日志和事件。

```python
def get_logs(self, sandbox_id: str) -> LogsResult
```

---

### upload_file

上传文件到沙箱。

```python
def upload_file(
    self,
    sandbox_id: str,
    path: str,
    content: bytes,
    *,
    content_type: str = "application/octet-stream",
) -> None
```

---

### download_file

从沙箱下载文件。

```python
def download_file(self, sandbox_id: str, path: str) -> bytes
```

---

### wait_for_ready

等待沙箱就绪。

```python
def wait_for_ready(
    self,
    sandbox_id: str,
    *,
    poll_interval: float = 2.0,
    timeout: float = 300.0,
) -> Sandbox
```

**异常:**

- `TimeoutError` - 等待超时
- `SandboxFailedError` - 沙箱启动失败

---

## 模板服务 (TemplateService)

### create

创建新模板。

```python
def create(self, request: CreateTemplateRequest) -> Template
```

**异常:** `ConflictError` - 模板已存在

---

### list

获取模板列表。

```python
def list(
    self,
    *,
    tag: Optional[str] = None,
    search: Optional[str] = None,
    page: int = 1,
    page_size: int = 20,
) -> TemplateListResult
```

---

### get

获取指定模板详情。

```python
def get(self, name: str) -> Template
```

---

### update

更新模板（创建新版本）。

```python
def update(self, name: str, request: UpdateTemplateRequest) -> Template
```

---

### delete

删除模板。

```python
def delete(self, name: str) -> None
```

---

### list_versions

获取模板的所有版本。

```python
def list_versions(self, name: str) -> VersionListResult
```

---

### get_version

获取模板的指定版本。

```python
def get_version(self, name: str, version: int) -> TemplateVersion
```

---

### rollback

回滚模板到指定版本。

```python
def rollback(
    self,
    name: str,
    target_version: int,
    *,
    changelog: Optional[str] = None,
) -> RollbackResult
```

---

### export_yaml

导出模板为 YAML 格式。

```python
def export_yaml(
    self,
    name: str,
    *,
    version: Optional[int] = None,
) -> bytes
```

---

## 预拉取服务 (PrepullService)

### create

创建镜像预拉取任务。

```python
def create(
    self,
    image: str,
    *,
    timeout: int = 600,
) -> PrepullTask
```

---

### create_for_template

为模板创建预拉取任务。

```python
def create_for_template(self, template_name: str) -> PrepullTask
```

---

### list

获取预拉取任务列表。

```python
def list(
    self,
    *,
    image: Optional[str] = None,
    status: Optional[PrepullStatus] = None,
) -> list[PrepullTask]
```

---

### get

获取指定预拉取任务。

```python
def get(self, task_id: str) -> PrepullTask
```

---

### delete

删除预拉取任务。

```python
def delete(self, task_id: str) -> None
```

---

### wait_for_completion

等待预拉取任务完成。

```python
def wait_for_completion(
    self,
    task_id: str,
    *,
    poll_interval: float = 5.0,
    timeout: float = 1800.0,
) -> PrepullTask
```

---

## 导入/导出服务 (ImportExportService)

### import_templates

从 YAML 导入模板。

```python
def import_templates(
    self,
    yaml_content: bytes | str,
    *,
    strategy: ImportStrategy = ImportStrategy.CREATE_OR_UPDATE,
    auto_prepull: bool = False,
) -> ImportResult
```

**参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `yaml_content` | `bytes \| str` | YAML 内容 |
| `strategy` | `ImportStrategy` | 导入策略 |
| `auto_prepull` | `bool` | 是否自动预拉取镜像 |

---

### export_all

导出所有模板为 YAML。

```python
def export_all(
    self,
    *,
    tag: Optional[str] = None,
    names: Optional[list[str]] = None,
) -> bytes
```

---

## 数据模型

### Sandbox

```python
class Sandbox(BaseModel):
    id: str
    image: str
    cpu: str
    memory: str
    ttl: int
    env: Optional[dict[str, str]]
    status: SandboxStatus
    template: Optional[str]
    template_version: Optional[int]
    created_at: datetime
    expires_at: datetime
    access_token: Optional[str]
    access_url: Optional[str]
```

### SandboxStatus

```python
class SandboxStatus(str, Enum):
    PENDING = "pending"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    TERMINATING = "terminating"
    UNKNOWN = "unknown"
```

### SandboxOverrides

```python
class SandboxOverrides(BaseModel):
    cpu: Optional[str]
    memory: Optional[str]
    ttl: Optional[int]
    env: Optional[dict[str, str]]
```

### ExecResult

```python
class ExecResult(BaseModel):
    exit_code: int
    stdout: str
    stderr: str
```

### Template

```python
class Template(BaseModel):
    id: str
    name: str
    display_name: Optional[str]
    description: Optional[str]
    tags: Optional[list[str]]
    author: Optional[str]
    is_public: bool
    latest_version: int
    created_at: datetime
    updated_at: datetime
    spec: Optional[TemplateSpec]
```

### TemplateSpec

```python
class TemplateSpec(BaseModel):
    image: str
    command: Optional[list[str]]
    args: Optional[list[str]]
    resources: ResourceSpec
    ttl: int
    env: Optional[dict[str, str]]
    startup_script: Optional[str]
    startup_timeout: int
    files: Optional[list[FileSpec]]
    readiness_probe: Optional[ProbeSpec]
    network: Optional[NetworkSpec]
```

### PrepullTask

```python
class PrepullTask(BaseModel):
    id: str
    image: str
    status: PrepullStatus
    progress: Optional[PrepullProgress]
    template: Optional[str]
    error: Optional[str]
    started_at: datetime
    completed_at: Optional[datetime]
```

### PrepullStatus

```python
class PrepullStatus(str, Enum):
    PENDING = "pending"
    PULLING = "pulling"
    COMPLETED = "completed"
    FAILED = "failed"
```

### ImportStrategy

```python
class ImportStrategy(str, Enum):
    CREATE_ONLY = "create-only"
    UPDATE_ONLY = "update-only"
    CREATE_OR_UPDATE = "create-or-update"
```

---

## 异常

### LiteBoxdError

SDK 基础异常类。

### APIError

API 错误响应。

```python
class APIError(LiteBoxdError):
    status_code: int
    message: str
```

### NotFoundError

资源不存在 (HTTP 404)。

### ConflictError

资源冲突 (HTTP 409)。

### BadRequestError

请求无效 (HTTP 400)。

### UnauthorizedError

未授权 (HTTP 401)。

### InternalServerError

服务器内部错误 (HTTP 500)。

### TimeoutError

操作超时。

### SandboxFailedError

沙箱启动失败。

```python
class SandboxFailedError(LiteBoxdError):
    sandbox_id: str
```

### PrepullFailedError

预拉取任务失败。

```python
class PrepullFailedError(LiteBoxdError):
    task_id: str
    error: str
```
