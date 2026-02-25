# LiteBoxd 沙箱元数据管理设计方案

## 1. 背景与现状

当前沙箱生命周期核心路径是“直接操作/读取 K8s Pod”：

- 创建：`backend/internal/service/sandbox.go` 调用 `k8sClient.CreatePod()`。
- 查询：`Get/List` 直接 `GetPod/ListPods`，再 `podToSandbox()` 转换。
- 过期清理：`StartTTLCleaner()` 定时扫描 Pod 注解并删除。
- 网关鉴权：`backend/internal/gateway/middleware.go` 直接从 Pod annotation 读取 token。
- 本地 SQLite 目前只保存模板与预拉取（`backend/internal/store/sqlite.go` 里的 `templates/template_versions/image_prepulls`），没有 `sandboxes` 相关持久化表。

这会导致：

- 控制面无“自有账本”，集群短时不可达时 `List/Get` 直接退化为失败。
- 沙箱历史不可追溯（删除后即丢失），无法满足审计和对账。
- 无法区分“期望状态”和“观测状态”，外部改动（手工删 Pod）难以治理。
- 网关每次请求都依赖集群查询 token，控制面耦合度高。

## 2. 目标与非目标

## 2.1 目标

1. LiteBoxd 建立并维护**沙箱元数据主存储（DB）**。
2. 集群状态通过 Watch + 周期扫描持续回灌，实现**最终一致**。
3. 支持**对账**（差异识别、修复动作、可追踪记录）。
4. 保持现有沙箱 API 基本兼容，分阶段切换，不一次性重构。
5. 为后续统计、计费、审计、SLO 打基础。

## 2.2 非目标

1. 本阶段不引入多租户权限模型。
2. 本阶段不更换 SQLite（先兼容当前部署形态）。
3. 本阶段不改造沙箱运行时形态（仍为 K8s Pod）。

## 3. 设计原则

1. **DB 为控制面真相源（source of truth）**：生命周期元数据、期望状态、历史事件以 DB 为准。
2. **K8s 为运行态真相源**：Pod phase/IP/UID 等实时态来自集群观测。
3. **双通道同步**：事件流（Watch）+ 周期全量扫描（Reconcile）并存，避免漏事件。
4. **直接切换关键路径**：不保留前向兼容分支与 fallback 路径，直接按目标架构落地。
5. **可审计**：每次状态变更与修复动作留痕。

## 4. 总体架构

```
API/Service (Create/Get/List/Delete)
        |
        | read/write sandbox metadata
        v
SQLite: sandboxes + status_history + reconcile_runs/items
        ^
        | update observed state
K8s Watcher (Pod add/update/delete) + Periodic Reconciler (full scan)
        |
        v
Kubernetes (pods in sandbox namespace)
```

关键点：

- **写路径**：API 先写 DB 的生命周期记录，再执行 K8s 动作并回写结果。
- **读路径**：`Get/List` 默认读 DB；若状态陈旧可异步触发刷新。
- **对账路径**：周期任务比较 DB 和 K8s，记录 drift 并按策略修复。

## 5. 数据模型设计

## 5.1 sandboxes（主表）

```sql
CREATE TABLE IF NOT EXISTS sandboxes (
  id TEXT PRIMARY KEY,

  -- 期望配置（来自模板 + overrides）
  template_name TEXT NOT NULL,
  template_version INTEGER NOT NULL,
  image TEXT NOT NULL,
  cpu TEXT NOT NULL,
  memory TEXT NOT NULL,
  ttl INTEGER NOT NULL,
  env_json TEXT NOT NULL DEFAULT '{}',

  -- 生命周期状态
  desired_state TEXT NOT NULL DEFAULT 'active',   -- active|deleted
  lifecycle_status TEXT NOT NULL,                 -- creating|pending|running|succeeded|failed|terminating|deleted|lost|unknown
  status_reason TEXT NOT NULL DEFAULT '',

  -- 集群观测态
  cluster_namespace TEXT NOT NULL,
  pod_name TEXT NOT NULL,
  pod_uid TEXT NOT NULL DEFAULT '',
  pod_phase TEXT NOT NULL DEFAULT '',
  pod_ip TEXT NOT NULL DEFAULT '',
  last_seen_at TIMESTAMP,

  -- 访问元数据（令牌密文存储）
  access_token_ciphertext TEXT NOT NULL,
  access_token_nonce TEXT NOT NULL,
  access_token_key_id TEXT NOT NULL,
  access_token_sha256 TEXT NOT NULL,
  access_url TEXT NOT NULL,

  -- 时间字段
  created_at TIMESTAMP NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sandboxes_lifecycle_status ON sandboxes(lifecycle_status);
CREATE INDEX IF NOT EXISTS idx_sandboxes_desired_state ON sandboxes(desired_state);
CREATE INDEX IF NOT EXISTS idx_sandboxes_expires_at ON sandboxes(expires_at);
CREATE INDEX IF NOT EXISTS idx_sandboxes_last_seen_at ON sandboxes(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_sandboxes_template_name ON sandboxes(template_name);
CREATE INDEX IF NOT EXISTS idx_sandboxes_access_token_sha256 ON sandboxes(access_token_sha256);
```

说明：

- `desired_state` 和 `lifecycle_status` 分离，用于表达“用户意图 vs 实际执行进度”。
- `last_seen_at` 用于判定观测是否过期。
- `access_token` 不落明文。使用 `AES-GCM` 密文存储（`ciphertext + nonce + key_id`）+ `sha256` 用于鉴权比对。
- 加密密钥通过环境变量注入（例如 `SANDBOX_TOKEN_ENCRYPTION_KEY`），支持 `key_id` 轮转。

## 5.2 sandbox_status_history（状态变更流水）

```sql
CREATE TABLE IF NOT EXISTS sandbox_status_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sandbox_id TEXT NOT NULL,
  source TEXT NOT NULL,             -- api|watcher|reconciler|ttl_cleaner|system
  from_status TEXT NOT NULL,
  to_status TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TIMESTAMP NOT NULL,
  FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sandbox_status_history_sid_ct ON sandbox_status_history(sandbox_id, created_at DESC);
```

## 5.3 sandbox_reconcile_runs / sandbox_reconcile_items（对账记录）

```sql
CREATE TABLE IF NOT EXISTS sandbox_reconcile_runs (
  id TEXT PRIMARY KEY,
  trigger_type TEXT NOT NULL,       -- scheduled|manual|startup
  started_at TIMESTAMP NOT NULL,
  finished_at TIMESTAMP,
  total_db INTEGER NOT NULL DEFAULT 0,
  total_k8s INTEGER NOT NULL DEFAULT 0,
  drift_count INTEGER NOT NULL DEFAULT 0,
  fixed_count INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,             -- running|completed|failed
  error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS sandbox_reconcile_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  sandbox_id TEXT NOT NULL,
  drift_type TEXT NOT NULL,         -- missing_in_k8s|missing_in_db|status_mismatch|spec_mismatch
  action TEXT NOT NULL,             -- mark_lost|mark_deleted|alert_only|none
  detail TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL,
  FOREIGN KEY (run_id) REFERENCES sandbox_reconcile_runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sandbox_reconcile_items_run_id ON sandbox_reconcile_items(run_id);
```

## 6. 生命周期与状态机

建议统一状态（`lifecycle_status`）：

- `creating`：API 已接收并写入 DB，尚未成功创建 Pod。
- `pending`：Pod 已创建，处于 Pending。
- `running`：Pod Running 且 Ready（或满足就绪探针）。
- `succeeded`：一次性任务完成。
- `failed`：创建/运行失败。
- `terminating`：收到删除请求，正在删。
- `deleted`：确认已删除（DB 保留软删除记录）。
- `lost`：DB 中存在但集群长时间找不到对应 Pod。
- `unknown`：无法判断。

状态流（简化）：

`creating -> pending -> running -> (succeeded|failed|terminating)`

`terminating -> deleted`

`running/pending -> lost`（对账发现缺失且超过 10 分钟宽限期）

## 7. 核心流程设计

## 7.1 创建沙箱

1. 读取模板并合并 overrides。
2. 生成 `sandbox_id/access_token`，先将 token 加密并写入 `sandboxes`（`creating`）。
3. 调用 K8s 创建 Pod（annotation 继续带 token/template 信息）。
4. 成功则更新 DB：`pod_uid/pod_phase/last_seen_at`，状态到 `pending`。
5. 失败则更新 `failed` + `status_reason`，保留记录用于排障与对账。

## 7.2 查询（Get/List）

1. 默认直接读 `sandboxes` 表返回。
2. 若 `last_seen_at` 超过阈值（如 30s），后台异步触发单条 refresh。
3. 保持响应结构与当前 OpenAPI 兼容（字段不减）。

## 7.3 删除

1. 将 `desired_state` 标为 `deleted`，`lifecycle_status=terminating`。
2. 调用 K8s 删除 Pod。
3. 成功/NotFound 都可推进为 `deleted`（记录 reason）。
4. 软删除保留，供审计与对账。

## 7.4 TTL 清理

从“扫 Pod 注解”改为“扫 DB `expires_at` + `desired_state=active`”：

1. 命中过期记录后先标记 `terminating`。
2. 删除 Pod。
3. 最终标记 `deleted`，写状态流水。

## 7.5 网关鉴权

- 鉴权只查 DB，不查 Pod annotation。
- 请求 token 做 `sha256` 后与 `access_token_sha256` 比对；必要时可解密密文做二次校验。
- 不设计 fallback 到 K8s，不保留前向兼容分支。

## 8. 对账机制

## 8.1 触发方式

1. 定时（建议 1~5 分钟）。
2. 服务启动后执行一次全量。
3. 手动触发（本期提供对外 API）。

## 8.2 核心对账规则

1. DB 有、K8s 无：
   - `terminating/deleted` 直接收敛为 `deleted`。
   - 其他状态超过 10 分钟宽限期后标记 `lost`。
2. K8s 有、DB 无：
   - 记录 `missing_in_db`，默认 `alert_only`（仅识别 `app=liteboxd` + `sandbox-id` 标签），不自动补录。
3. 两边都有但状态不一致：
   - 以 K8s phase 更新 `pod_phase/lifecycle_status`，并记录 `status_mismatch`。
4. 规格不一致（可选）：
   - 记录 `spec_mismatch`，默认告警不自动修复。

## 8.3 对账产物

- 每次 run 产出统计（drift/fixed/error）。
- 每条 drift 产出 item 记录，支持后续审计与报表。

## 9. 与现有代码对齐的改造点

建议按以下文件范围演进：

- `backend/internal/store/sqlite.go`：新增 `sandboxes`/history/reconcile 表。
- `backend/internal/store/`：新增 `sandbox.go`（Repository）。
- `backend/internal/service/sandbox.go`：改为 DB 主读写 + K8s 动作执行。
- `backend/internal/service/`：新增 `sandbox_reconciler.go`。
- `backend/internal/gateway/middleware.go`：鉴权从 Pod annotation 改为 DB。
- `backend/cmd/server/main.go`：注册 reconciler 与新的 TTL 清理入口。

## 10. API 与兼容性策略

1. 现有 `POST/GET/LIST/DELETE /sandboxes` 不改路径。
2. 返回字段保持兼容（`id/image/cpu/memory/ttl/status/created_at/expires_at/template/templateVersion/accessToken/accessUrl`）。
3. 本期新增并开放对账 API，至少包含：
   - `POST /sandboxes/reconcile`（手动触发一次对账）
   - `GET /sandboxes/reconcile/runs`（查看对账任务列表）
   - `GET /sandboxes/reconcile/runs/{id}`（查看单次对账详情）
4. 新增 API 必须同步 `api/openapi.yaml`。

## 11. 分阶段落地计划

## Phase 1：建模与直接切换

1. 建表与 `SandboxStore`（含 token 密文存储字段）。
2. `Create/Get/List/Delete/TTL` 全部切到 DB 主路径。
3. 网关鉴权切到 DB（不保留 Pod annotation 鉴权逻辑）。

验收：核心生命周期与鉴权路径不再依赖旧逻辑分支。

## Phase 2：同步与对账闭环

1. 引入 Watcher 更新 `last_seen_at/pod_phase/pod_ip`。
2. 周期全量 reconcile（含 10 分钟 `lost` 判定）。
3. 落库 reconcile run/item，并执行 `missing_in_db=alert_only`。

验收：可稳定识别并记录漂移，且不产生自动补录副作用。

## Phase 3：对外 API 与可观测

1. 开放对账 API（触发、列表、详情）。
2. 补齐日志与指标（run 数、drift 数、修复数、告警数）。
3. 完成 OpenAPI 与回归测试。

验收：用户可通过 API 查询和触发对账，并获得可追踪结果。

## 12. 风险与缓解

1. 直切换回归风险：
   - 缓解：先写 DB `creating`，失败回写 `failed`，并以集成测试覆盖创建/删除/鉴权主路径。
2. Watch 漏事件：
   - 缓解：周期全量 reconcile 兜底。
3. token 安全：
   - 缓解：本期直接上密文存储 + 哈希比对，不走明文过渡。
4. 历史数据膨胀：
   - 缓解：状态流水与对账明细设置保留策略（例如 30~90 天归档）。

## 13. 已确认决策

1. `missing_in_db` 默认动作：仅告警（`alert_only`），不自动补录。
2. `lost` 状态判定宽限期：10 分钟。
3. `access_token`：本期切到加密存储。
4. 对账能力：本期对外提供 API。

---

创建日期：2026-02-24
文档状态：草案（待评审）
