# LiteBoxd Code Interpreter 沙箱支持方案

## 1. 概述

### 1.1 背景

LiteBoxd 目前提供基于模板的沙箱创建、命令执行、文件上传下载与网络访问能力。为了支持类 Code Interpreter 的交互式编程体验，需要一个面向多语言的运行时环境、可保持上下文的会话机制、可流式返回输出的执行通道，以及对沙箱内文件系统的完整操作能力。

### 1.2 设计目标

- **多语言执行**：支持 Python/Java/JavaScript/TypeScript/Go/Bash 等语言。
- **会话保持**：支持长生命周期会话，以便复用变量与状态。
- **流式输出**：执行过程支持 SSE 流式输出与结构化事件。
- **文件管理**：支持文件 CRUD、目录遍历与下载。
- **安全隔离**：复用 LiteBoxd 的网络隔离与访问令牌机制。
- **易于接入**：与现有模板系统和网关访问路径无缝对齐。

### 1.3 非目标

- 不提供在沙箱内构建自定义镜像的能力。
- 不提供跨沙箱共享会话或共享文件的能力。
- 不引入额外的多租户计费体系。

## 2. 使用场景

1. **交互式代码执行**：执行多语言代码并获取 stdout/stderr 与结果对象。
2. **多轮对话式编程**：保持会话上下文，连续执行多个代码片段。
3. **文件处理**：上传数据文件、生成图表或输出结果文件并下载。
4. **Jupyter 兼容**：提供基于 Jupyter 内核的语言执行支持。

## 3. 总体架构

```
用户/SDK
    │
    ▼
LiteBoxd API
    │
    ▼
网关服务 (令牌认证 + 转发)
    │
    ├──► /sandbox/{id}/port/44772  运行时守护进程 (HTTP/SSE)
    └──► /sandbox/{id}/port/44771  Jupyter Server

Sandbox Pod
┌────────────────────────────────────────────┐
│ Code Interpreter 镜像                       │
│  ├── 运行时守护进程 (HTTP/SSE)              │
│  ├── Jupyter Server                         │
│  └── 多语言运行时 + 内核                     │
└────────────────────────────────────────────┘
```

## 4. 运行时组件设计

### 4.1 Code Interpreter 镜像

镜像提供统一的多语言执行环境，包含：

- 多语言运行时与多版本管理脚本
- Jupyter Server 与多语言内核
- 运行时守护进程二进制与启动脚本
- 默认工作目录 `/workspace`

建议镜像内置以下环境变量：

- `WORKSPACE_DIR=/workspace`
- `JUPYTER_HOST=0.0.0.0`
- `JUPYTER_PORT=44771`
- `JUPYTER_TOKEN`（随机或由模板传入）
- `RUNTIME_PORT=44772`
- `RUNTIME_TOKEN`（可选，优先使用 LiteBoxd 访问令牌）

内置模板：

- 模板名：`code-interpreter`
- 模板文件：`templates/code-interpreter.yml`
- 镜像：`docker.cnb.cool/fslongjin/liteboxd/code-interpreter:v0.1.0`

### 4.2 运行时守护进程

运行时守护进程负责将 HTTP 请求转化为实际的执行动作，提供：

- **代码执行**：基于 Jupyter Kernel 或内置执行器
- **命令执行**：同步与后台命令
- **会话管理**：创建、复用、关闭会话
- **文件管理**：文件/目录的列举、上传、下载、删除
- **可观测性**：系统指标与运行时事件

建议支持 SSE 事件格式，以便实时输出：

```json
{
  "type": "stdout",
  "session_id": "sess-123",
  "data": "hello\n"
}
```

### 4.3 Jupyter Server

Jupyter Server 为语言内核提供状态保持能力。运行时守护进程通过 WebSocket 与 Jupyter 交互，对外提供统一的 REST/SSE API。

## 5. 访问与协议

### 5.1 网关访问路径

基于已有网关路径，客户端无需新增入口：

- 运行时守护进程：`{access_url}/port/44772`
- Jupyter Server：`{access_url}/port/44771`

请求头统一使用 LiteBoxd 访问令牌：

```
X-Access-Token: {sandbox_access_token}
```

### 5.2 运行时 API 草案

#### 代码执行

```
POST /code
```

请求：

```json
{
  "language": "python",
  "code": "print('hello')",
  "session_id": "sess-123",
  "timeout": 30
}
```

响应（SSE）：

```
event: stdout
data: hello
```

#### 会话管理

```
POST /sessions
DELETE /sessions/{session_id}
```

#### 文件管理

```
GET /files?path=/workspace
POST /files/upload
GET /files/download?path=/workspace/output.txt
```

#### 指标

```
GET /metrics
GET /metrics/watch
```

## 6. 安全与隔离

- 复用 LiteBoxd 默认的网络隔离策略，默认拒绝出网。
- 通过网关令牌认证保护运行时 API 与 Jupyter 端口。
- 限制文件访问范围为 `/workspace`，禁止访问敏感路径。
- 使用资源配额与 TTL，防止资源耗尽。

## 7. 可观测性与运维

- 运行时守护进程提供 `/metrics` 与 `/metrics/watch`。
- LiteBoxd 侧继续提供 `logs` 与 `events` 接口用于排障。
- 建议在镜像内落地结构化日志，便于后续收集。

## 8. 实现计划

### Phase 1：基础能力

- 构建 Code Interpreter 镜像（多语言 + Jupyter + 运行时守护进程）
- 上线模板与预拉取配置
- 通过网关路径访问运行时守护进程

### Phase 2：交互体验

- SSE 事件模型稳定化
- 会话复用与超时回收
- 文件上传下载与目录管理 API

### Phase 3：性能与规模

- 常驻热池或镜像预热方案
- 工作目录持久化选项
- 细粒度配额与并发限制
