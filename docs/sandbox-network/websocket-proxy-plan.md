# 网关 WebSocket 透传方案

## 目标

- 支持通过网关对沙箱内服务的 WebSocket 连接透传
- 与现有 HTTP 代理、令牌认证和路径路由保持一致
- 同时兼容直连 Pod IP 与 K8s API Server Proxy 两种模式

## 入口与路由

- 请求路径：`/api/v1/sandbox/{sandbox-id}/port/{port}/{path}`
- 认证方式：`X-Access-Token` Header（沿用现有网关中间件）
- WebSocket Upgrade 由网关识别并转换为双向流转发

## 设计概览

### 透传流程

1. 中间件完成 sandbox-id、端口校验、令牌认证
2. 识别 Upgrade 请求并停止走 HTTP ReverseProxy
3. 建立客户端 WebSocket 连接
4. 建立到后端服务的 WebSocket 连接
5. 启动双向转发，直到任一端关闭

### 两种后端连接模式

1. 直连 Pod IP
   - 目标地址：`ws://{podIP}:{port}/{path}`
   - 由网关负责路径裁剪与 Query 透传
2. K8s API Server Proxy
   - 目标地址：`wss://{apiserver}/api/v1/namespaces/liteboxd/pods/sandbox-{id}:{port}/proxy/{path}`
   - 使用与 k8s client 一致的 TLS 配置
   - Authorization Header 使用 kubeconfig 中的凭据

## 详细方案

### 1. WebSocket 识别

- 判断 `Connection: upgrade` 且 `Upgrade: websocket`
- 对非 WS 请求保持现有 `ReverseProxy` 逻辑不变

### 2. 路径处理

- 入口路径：`/api/v1/sandbox/{id}/port/{port}/{path}`
- 直连模式将路径裁剪为 `{path}`，并保留 Query
- K8s Proxy 模式将路径改写为 `/api/v1/namespaces/liteboxd/pods/sandbox-{id}:{port}/proxy/{path}`

### 3. 客户端升级

- 使用 gorilla/websocket 将客户端连接升级
- 只允许 `X-Access-Token` 完成认证后才升级
- 保留 `Sec-WebSocket-Protocol` 子协议头

### 4. 上游连接

#### 4.1 直连 Pod IP

- 使用 `websocket.Dialer` 连接目标 `ws://` 地址
- 转发必要的 Header（如 `Sec-WebSocket-Protocol`）
- 不转发 `X-Access-Token`

#### 4.2 K8s Proxy 模式

- 使用 kubeconfig TLS 配置与 API Server 建立 `wss://` 连接
- `Authorization` Header 设置为 Bearer Token
- 保持与现有 K8s Proxy 路径一致

### 5. 双向转发

- 启动两个 goroutine
  - client -> upstream
  - upstream -> client
- 发生错误或关闭时，关闭双方连接
- 保持帧类型一致（Text/Binary/Close/Ping/Pong）

### 6. 超时与资源管理

- 复用网关 `RequestTimeout` 作为读写超时基线
- 连接空闲时保活：定期 Ping
- 连接关闭时及时回收 goroutine

### 7. 错误处理与状态码

- 令牌校验失败：401
- Sandbox 不存在：404
- 上游连接失败：502
- 其他内部错误：503

## 兼容性与约束

- 不支持非 WebSocket 的 TCP 隧道
- 仅适配 HTTP/WS 端口访问
- 依赖 NetworkPolicy 允许网关访问沙箱端口

## 验收标准

- HTTP 请求正常转发不受影响
- WebSocket 连接可建立并透传双向消息
- 直连模式与 K8s Proxy 模式均可用
- 认证失败时不发生 Upgrade

## 测试建议

- 基础连接：ws echo 服务
- 子协议透传：携带 `Sec-WebSocket-Protocol`
- 断连场景：客户端/服务端主动关闭
- K8s Proxy 模式：开启 `DEV_USE_K8S_PROXY` 运行验证
