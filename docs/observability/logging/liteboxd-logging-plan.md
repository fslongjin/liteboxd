# liteboxd 日志投递方案（K8s + 本地开发）

## 背景

当前 `liteboxd server` 和 `liteboxd gateway` 主要通过 `stdout` 打印日志（含 `fmt/log/gin.Logger` 默认输出），这在本地可用，但在生产环境需要满足：

- 日志结构化（便于检索、告警、分析）
- 可投递到腾讯云 CLS（或其他日志服务）
- 当未配置远端日志服务时，仍有可用的本地滚动日志兜底
- 方案不要过重，不引入过多组件或复杂运维成本

## 设计目标

- **统一格式**：server/gateway 统一 JSON 结构日志
- **分层解耦**：应用负责“产生日志”，采集层负责“投递到哪里”
- **双场景兼容**：K8s 上线和 `make run-backend/run-gateway` 本地开发都顺滑
- **降级可用**：未配置 CLS 时，本地可落盘并滚动

## 推荐方案（简单且可演进）

采用“两层模型”：

1. **应用层（liteboxd）**
   - 统一使用结构化 logger（建议 Go `slog` JSON）
   - 默认输出到 `stdout`
   - 可选同时输出到本地滚动文件（用于开发机/单机调试）
2. **采集与投递层**
   - **K8s 环境**：使用 DaemonSet 日志采集器（推荐 Fluent Bit / Vector）采集容器 `stdout`，转发到 CLS
   - **本地环境**：可不启采集器，直接看控制台；如需保留日志则由应用写本地滚动文件

这个方案的核心是：**应用不直接耦合 CLS SDK**，避免在业务代码里处理重试、批量、鉴权等复杂逻辑。

## 为什么这个方案最合理

- 对现状改造小：保留 `stdout` 主路径，不破坏 K8s 日志生态
- 对 K8s 最友好：容器日志天然由平台采集，后续切换 CLS/Loki/ES 成本低
- 本地开发体验好：`make run-*` 仍然能直接看日志，且可选落盘滚动
- 风险低：远端投递链路异常时，应用不被阻塞；至少有 stdout/file 可用

## 日志输出与投递策略

### 1) 应用层输出策略

为 server/gateway 增加统一日志配置（环境变量）：

- `LOG_LEVEL`：`debug|info|warn|error`（默认 `info`）
- `LOG_FORMAT`：`json|text`（默认 `json`）
- `LOG_OUTPUT`：`stdout|stdout,file|file`（默认 `stdout`）
- `LOG_FILE_PATH`：本地文件路径（默认 `./logs/liteboxd.log`）
- `LOG_FILE_MAX_SIZE_MB`：单文件大小（默认 `100`）
- `LOG_FILE_MAX_BACKUPS`：保留份数（默认 `7`）
- `LOG_FILE_MAX_AGE_DAYS`：保留天数（默认 `7`）

字段规范建议至少包含：

- `ts`（RFC3339 时间）
- `level`
- `service`（`liteboxd-server` / `liteboxd-gateway`）
- `component`（handler/service/k8s/gateway-proxy 等）
- `msg`
- `request_id`（HTTP 请求链路）
- `method` / `path` / `status` / `latency_ms`（访问日志）
- `sandbox_id`（有上下文时）
- `error`（错误信息）

### 2) K8s 投递到 CLS

- 部署日志采集器 DaemonSet（Fluent Bit / Vector 均可）
- 采集 `liteboxd-api` 与 `liteboxd-gateway` Pod 容器日志
- 按 `namespace=liteboxd-system`、`app` label 做路由
- 输出目标配置为 CLS Topic

建议：

- 使用 Secret 保存 CLS 凭据，避免明文写在 ConfigMap
- 采集器增加缓冲与重试（避免短时网络抖动丢日志）
- 仅采集 JSON 日志，降低解析复杂度

### 3) 未配置 CLS 时的兜底

- **K8s**：继续保留 `stdout`（可用 `kubectl logs`，且由容器运行时负责日志轮转）
- **本地开发**：启用 `LOG_OUTPUT=stdout,file`，落盘到 `./logs/` 并轮转

> 说明：在 K8s Pod 内仅依赖“容器内文件”做长期日志存储不可靠（Pod 重建会丢）；生产主路径应是 stdout + 集群采集。

## 环境推荐默认值

### 本地开发（make run-*）

- `LOG_LEVEL=debug`
- `LOG_FORMAT=text`（可读性优先）或 `json`（联调采集器时）
- `LOG_OUTPUT=stdout,file`
- `LOG_FILE_PATH=./logs/liteboxd.log`

### K8s 生产

- `LOG_LEVEL=info`
- `LOG_FORMAT=json`
- `LOG_OUTPUT=stdout`
- 由 DaemonSet 采集器转发到 CLS

## 最小实施步骤（建议分两步）

### Step 1（必须，低复杂度）

- 抽一个 `internal/logx`（或同类）统一 logger 初始化
- 替换 server/gateway 启动与关键路径日志输出（`fmt/log/gin.Logger` -> 统一结构化日志）
- 增加基础请求日志中间件（统一 `request_id`、耗时、状态码）
- 本地支持滚动文件输出（如 `lumberjack`）

### Step 2（上线配套）

- 在集群部署 Fluent Bit/Vector 到 `liteboxd-system`
- 配置输出到 CLS（Secret 注入凭据）
- 增加最小告警（例如 `error` 级别计数阈值）

## 备选方案对比（为何不选）

### A. 应用直接写 CLS SDK

- 优点：链路看起来直达
- 缺点：应用耦合重、重试/批处理/限流复杂、切换日志后端成本高
- 结论：不推荐作为第一版

### B. 仅文件日志 + filebeat sidecar

- 优点：兼容传统部署
- 缺点：K8s 下会增加卷与 sidecar 管理复杂度
- 结论：不如 stdout + DaemonSet 简洁

## 验收标准

- server/gateway 日志可稳定输出 JSON，关键字段齐全
- 本地未配置 CLS 时，`./logs/` 能自动轮转且不影响控制台输出
- K8s 上配置 CLS 后，可按 `service/request_id/status` 检索日志
- 关闭 CLS 或网络异常时，应用服务不因日志链路阻塞

