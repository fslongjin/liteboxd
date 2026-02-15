# LiteBoxd 控制面上 K3s + 热升级改造设计（评审稿）

## 1. 目标与范围

### 1.1 目标

在保持现有沙箱能力的前提下，实现：

1. `liteboxd` 本体（API Server + Gateway）也部署在 k3s。
2. 控制面与沙箱面使用**不同 namespace 隔离**。
3. 支持热升级，升级期间：
   - 不丢失控制面元数据（模板、预拉取状态等）；
   - 不影响已运行沙箱；
   - 尽量避免会话断开（本期采用“排空不强踢”策略）。

### 1.2 本次设计覆盖

- 架构与部署改造
- 代码重构边界
- 升级与会话连续性策略
- 分阶段实施方案

不在本次首批强制范围：

- 全量多租户权限模型（RBAC 细粒度多租户）
- 跨集群调度

## 1.3 已确认决策（本轮）

1. 数据层路线：`SQLite + PVC` 过渡方案（暂不直接切 Postgres）。
2. 会话目标：仅“排空不强踢”（不做会话级恢复）。
3. namespace 命名：采用 `liteboxd-system` / `liteboxd-sandbox`。

---

## 2. 现状调研结论（基于当前代码）

## 2.1 关键现状

1. 现有 `server`/`gateway` 默认是“本地进程连远程 K8s”模型。
   - `backend/cmd/server/main.go`
   - `backend/cmd/gateway/main.go`
2. 沙箱 namespace 在代码中是硬编码常量 `liteboxd`。
   - `backend/internal/k8s/client.go` 中 `SandboxNamespace = "liteboxd"`
3. `gateway` 有 K8s Deployment（`deploy/gateway.yaml`），但 `server` 没有 k8s 部署清单。
4. 元数据存储是本地 SQLite 文件（`DATA_DIR/liteboxd.db`），当前不具备多副本共享能力。
   - `backend/internal/store/sqlite.go`
5. 交互终端是 WebSocket -> backend -> SPDY exec 的直连桥接，进程重启会断。
   - `backend/internal/handler/sandbox.go`
   - `backend/internal/service/sandbox.go`
6. backend 目前没有优雅停机（无 signal + drain）；gateway 有基本 `http.Server.Shutdown`，但无连接级 drain。
7. 网络策略和网关路径均耦合单一 namespace，且假设 gateway 与 sandbox 在同一 namespace。
   - `backend/internal/k8s/network_policy.go`
   - `backend/internal/gateway/proxy.go`

## 2.2 与目标的主要差距

### A. 部署层差距

1. 缺少 `server` 的 k8s 部署（Service/Deployment/RBAC/PDB 等）。
2. 控制面与沙箱面 namespace 未解耦。
3. 目前依赖 `KUBECONFIG` 文件路径，in-cluster 模式不友好（默认会回退 `~/.kube/config`）。
4. `backend/Dockerfile` 当前 `CGO_ENABLED=0` 构建 `cmd/server`，与 SQLite 依赖（`mattn/go-sqlite3`）冲突，server 镜像构建链路需修正。

### B. 数据层差距

1. SQLite 本地文件仅适合单实例，无法支撑无损滚动升级的多副本切换。
2. 数据持久化清单（PVC/备份）未定义。

### C. 热升级与会话层差距

1. backend 无优雅停机 + 无连接排空能力。
2. gateway 未跟踪活跃 WebSocket 连接，升级会中断长连接。
3. 交互终端会话无“重连恢复”语义。

### D. 运维与安全差距

1. 缺少最小权限 RBAC（当前部署更像依赖 kubeconfig 高权限凭据）。
2. 缺少升级就绪探针/排空状态接口。
3. 现有 `deploy/gateway.yaml` 中 PDB 同时设置了 `minAvailable` 和 `maxUnavailable`（k8s 不允许同时设置），部署清单需先纠正。
4. 现有网络策略清单中端口范围写法（`3000-65535`）不符合 `NetworkPolicyPort` 标准格式，需改为 `port + endPort`。

---

## 3. 目标架构

## 3.1 Namespace 划分

- `liteboxd-system`：控制面（api-server、gateway、web、db、监控）
- `liteboxd-sandbox`：运行时沙箱 Pod + 运行时网络策略

## 3.2 组件拓扑（建议）

1. `liteboxd-api`（Deployment，过渡期 1 副本）
2. `liteboxd-gateway`（Deployment，>=2 副本）
3. `web`（可选）
4. `Ingress`（路径路由）
   - `/api/v1/sandbox/**` -> gateway
   - `/api/v1/**` -> api

> 注：当前已确定先走 SQLite+PVC，`liteboxd-api` 维持单副本；后续如需真正多副本无损升级，再评估迁移 Postgres。

---

## 4. 详细改造方案

## 4.1 配置与命名空间解耦（必须）

新增统一配置项：

- `SANDBOX_NAMESPACE`（默认 `liteboxd-sandbox`）
- `CONTROL_NAMESPACE`（默认 `liteboxd-system`）
- `K8S_IN_CLUSTER`（默认 `true`）
- `KUBECONFIG`（仅本地开发使用）

代码改造点：

1. `backend/internal/k8s/client.go`
   - 去掉硬编码 `SandboxNamespace` 常量依赖，改为 `ClientConfig` 注入。
2. `backend/internal/k8s/network_policy.go`
   - 策略作用 namespace、gateway 来源 namespace 改为可配置。
3. `backend/internal/gateway/proxy.go`
   - K8s proxy path 的 namespace 改为配置值。

## 4.2 In-Cluster 认证改造（必须）

当前逻辑是“有 `kubeconfigPath` 就走文件，否则走 `InClusterConfig`”。但入口默认会填 `~/.kube/config`，导致容器内默认并不会自动走 in-cluster。

改造：

1. `cmd/server`、`cmd/gateway`：
   - 若显式设置 `KUBECONFIG` 才走文件；
   - 否则默认 `rest.InClusterConfig()`。
2. 部署清单默认只依赖 ServiceAccount + RBAC，不再挂 kubeconfig secret。

## 4.3 存储层改造（建议按两阶段）

### 阶段 1（已选定）

- 继续 SQLite，但放 PVC。
- `liteboxd-api` 保持单副本。
- 可保证“重建不丢模板/预拉取元数据”，但滚动热升级能力有限。

### 阶段 2（推荐）

- 存储迁移到 Postgres（或等价外部 DB）。
- `liteboxd-api` 可多副本，支持标准 RollingUpdate。

需要改造：

1. `backend/internal/store/*`
   - 抽象 Repository 接口；
   - 新增 postgres 实现；
   - 初始化流程改成可选 `sqlite|postgres`。
2. 新增迁移机制（migrations）。

## 4.4 背景任务主从化（必须，若 API 多副本）

当前 TTL Cleaner、Prepull Status Updater 在每个进程都会启动（`backend/cmd/server/main.go`），多副本会重复执行。

改造：

1. 引入 leader election（Lease）或单独 `liteboxd-controller` 组件。
2. 只有 leader 执行：
   - TTL 清理
   - prepull 状态轮询
   - 历史清理任务

## 4.5 热升级与连接排空（必须）

## 4.5.1 API/Gateway 统一 drain 机制

新增：

1. `GET /readyz`：正常返回 200；draining 返回 503。
2. 收到 SIGTERM：
   - 标记 draining（探针失败，不再接收新流量）；
   - 等待活跃连接排空；
   - 到超时后退出。

## 4.5.2 WebSocket 连接跟踪

在 gateway 与 backend interactive exec 中增加连接管理器：

- 连接建立/关闭计数
- 停机时等待 `active==0` 或超时

Deployment 策略：

- `rollingUpdate.maxUnavailable: 0`
- `rollingUpdate.maxSurge: 1`
- `terminationGracePeriodSeconds` 适当拉长（例如 300~1800，根据业务）
- `PodDisruptionBudget`（仅设置 `minAvailable`，不要同时配 `maxUnavailable`）

## 4.6 会话不断开策略（分级）

### 级别 A（已选定，首期交付）

- 通过连接排空，尽量不强踢已有连接。
- 升级期间若连接自然结束，不影响新连接切到新 Pod。

### 级别 B（后续可选：终端可恢复）

针对 `/sandboxes/:id/exec/interactive`：

1. 引入“终端会话ID”概念；
2. 采用可重入 shell（如 tmux/screen 或等价会话代理）承载交互状态；
3. 客户端断线自动重连并 attach 同一会话。

说明：

- 对“任意业务 WebSocket”（通过 gateway 代理到用户应用）无法由平台无损迁移协议状态；
- 可做到的是**平台不主动切断 + 客户端重连策略**。

## 4.7 沙箱数据不丢失边界说明（必须明确）

1. 升级 `liteboxd` 不应删除/重建现有 sandbox Pod，因此沙箱内运行态和 `emptyDir` 数据不会因控制面升级直接丢失。
2. 但若 sandbox Pod 本身重建，`emptyDir` 仍会丢失（这是当前产品行为）。
3. 若你要求“沙箱 Pod 重建也保数据”，需额外引入 PVC 工作目录能力（这是另一个需求面）。

## 4.8 部署清单改造（必须）

新增或改造：

1. `deploy/system/`：
   - namespace
   - api Deployment/Service/PDB
   - gateway Deployment/Service/PDB
   - ServiceAccount/Role/RoleBinding
   - ConfigMap/Secret
2. `deploy/sandbox/`：
   - sandbox namespace
   - network policies（允许来自 `liteboxd-system` 中 gateway 的 ingress）
3. DB 部署（若选 postgres）
4. Ingress/Service 路由清单

---

## 5. 需要改造的主要代码文件（第一批）

1. `backend/cmd/server/main.go`
2. `backend/cmd/gateway/main.go`
3. `backend/internal/k8s/client.go`
4. `backend/internal/k8s/network_policy.go`
5. `backend/internal/gateway/config.go`
6. `backend/internal/gateway/proxy.go`
7. `backend/internal/handler/sandbox.go`
8. `backend/internal/service/sandbox.go`
9. `backend/internal/store/*`（若做 DB 抽象/迁移）
10. `deploy/*`（新增 system/sandbox 分层清单）
11. `api/openapi.yaml`（若新增/调整健康探针、会话相关 API）

---

## 6. 分阶段实施计划

## Phase 1：控制面上 K3s + namespace 解耦（必须）

交付：

1. server/gateway 均可 in-cluster 运行
2. `liteboxd-system` 与 `liteboxd-sandbox` 分离
3. 基础 RBAC + NetworkPolicy 跨 namespace 生效
4. 新部署清单可一键部署

## Phase 2：热升级基础能力（必须）

交付：

1. API server 增加优雅停机 + drain
2. gateway 增加连接跟踪与排空
3. Deployment/PDB/Probe 策略正确

## Phase 3：数据层升级（建议）

交付（当前采用方案 1）：

1. 过渡：SQLite + PVC + 单副本 API（当前选定）
2. 推荐：Postgres + 多副本 API

## Phase 4：会话连续性增强（建议）

交付：

1. （可选）交互终端可恢复机制
2. 前端/SDK 自动重连优化

---

## 7. 验收标准（建议）

1. 升级前后，已有 sandbox 列表与状态一致（无误删）。
2. 模板与 pre-pull 记录升级后可读取。
3. 滚动升级期间：
   - 新请求持续可用；
   - 已建立 websocket 不被立即踢断（按排空策略）；
4. 本期不要求“会话可恢复”；若后续启用该能力，再补充对应验收项。

---

## 8. 关键风险与规避

1. **SQLite 多副本写冲突**：避免直接多副本共享 SQLite；推荐迁移 Postgres。
2. **长连接导致 rollout 卡住**：为 drain 设置上限时长 + 运维强制开关。
3. **RBAC 过宽**：按最小权限拆分 Role（sandbox 资源、networkpolicy、pods/exec、jobs）。
4. **网络策略误配导致不可达**：上线前用 e2e 用例验证 gateway->sandbox、sandbox->dns、sandbox->internet 场景。

---

## 9. 已确认实施基线

1. 数据层路线：先 `SQLite+PVC`（过渡方案）。
2. 会话目标：仅“排空不强踢”（本期不引入会话恢复协议）。
3. namespace 命名：采用 `liteboxd-system` / `liteboxd-sandbox`。

---

## 10. 结论

要实现你提出的“`liteboxd` 也部署到 k3s + namespace 隔离 + 热升级不丢数据/会话不中断”，当前代码可以复用大部分业务能力，但必须补齐以下核心改造：

1. namespace/配置去硬编码
2. in-cluster 认证与 RBAC
3. 连接排空与优雅停机
4. 存储方案升级（至少 PVC，推荐 Postgres）
5.（后续可选）终端会话可恢复机制

你确认本方案后，我按 Phase 1 -> Phase 2 顺序开始实施，并在每个 phase 完成后给你可验证的部署与回归步骤。
