# 沙箱元数据后台查询功能设计

## 1. 背景与目标

当前系统已经将沙箱元数据、状态流水、对账记录落在 SQLite，但后台 Web 仅提供：

- 沙箱列表（基础字段）
- 沙箱详情（基础字段 + 日志/执行/文件）

缺失“管理面查询”能力：

- 不能按元数据维度筛选（如 `lifecycle_status/desired_state/template`）。
- 不能查看单沙箱状态流水（`sandbox_status_history`）。
- 对账记录仅有后端 API，Web 无统一查询入口。

本方案目标：在后台 Web 提供可审计、可追踪、可检索的“沙箱元数据记录查询能力”。

## 2. 现状调研结论

### 2.1 后端现状

已具备：

- `POST /sandboxes/reconcile`
- `GET /sandboxes/reconcile/runs`
- `GET /sandboxes/reconcile/runs/{id}`

当前不足：

- `GET /sandboxes` 不支持筛选和分页（仅返回 active 列表）。
- 无 `sandbox_status_history` 查询 API。
- 对账 item 只能通过 run 详情间接查，不支持跨 run 检索。

### 2.2 Web 现状

- 路由：`/sandboxes`、`/sandboxes/:id`、`/templates`。
- 无“对账记录”入口页。
- 沙箱详情页未展示元数据字段（`desired_state/lifecycle_status/pod_phase/last_seen_at/deleted_at/status_reason`）及状态流水。

## 3. 需求范围

## 3.1 本期范围（建议）

1. 元数据列表查询（含已删除记录）。
2. 沙箱详情中的状态流水查询。
3. 对账记录中心（run 列表、run 详情、手动触发）。

## 3.2 非目标

1. 不做自动修复策略编排界面。
2. 不做复杂 BI 报表与导出。
3. 不改动鉴权模型（沿用当前后端鉴权）。

## 4. 信息架构与页面设计

## 4.1 新增导航

- 一级导航新增：`元数据记录`（路由建议：`/metadata`）。
- 子区域包含：
  - `沙箱元数据`（默认页）
  - `对账记录`

## 4.2 页面 A：沙箱元数据列表（/metadata/sandboxes）

展示字段（默认列）：

- `id`
- `template_name/template_version`
- `desired_state`
- `lifecycle_status`
- `pod_phase`
- `status_reason`
- `last_seen_at`
- `created_at`
- `expires_at`
- `deleted_at`

查询条件：

- `id`（精确或前缀）
- `template_name`
- `desired_state`（active/deleted）
- `lifecycle_status`（running/failed/lost/deleted/...）
- `created_at` 时间范围
- `deleted_at` 时间范围

交互要求：

- 默认按 `created_at DESC`。
- 支持分页（`page/page_size`）。
- 点击 `id` 跳转“元数据详情页”。

## 4.3 页面 B：沙箱元数据详情（/metadata/sandboxes/:id）

分区：

1. 基础元数据卡片（结构化展示）
2. 状态流水（`sandbox_status_history`）
3. 关联对账项（最近 N 条）

状态流水表字段：

- `created_at`
- `source`
- `from_status`
- `to_status`
- `reason`
- `payload_json`（可折叠 JSON 查看）

## 4.4 页面 C：对账记录中心（/metadata/reconcile）

上区：

- 手动触发按钮（调用 `POST /sandboxes/reconcile`）
- 运行列表（run）

run 列表字段：

- `id`
- `trigger_type`
- `started_at/finished_at`
- `total_db/total_k8s`
- `drift_count/fixed_count`
- `status/error`

下区：

- 选中某 run 后展示 item 列表（或右侧抽屉）。

item 字段：

- `sandbox_id`
- `drift_type`
- `action`
- `detail`
- `created_at`

## 5. API 设计（用于 Web 查询）

## 5.1 复用现有 API

- `POST /sandboxes/reconcile`
- `GET /sandboxes/reconcile/runs`
- `GET /sandboxes/reconcile/runs/{id}`

## 5.2 新增 API（本期建议）

### A. 元数据列表查询

`GET /sandboxes/metadata`

Query 参数：

- `id`（可选）
- `template`（可选）
- `desired_state`（可选）
- `lifecycle_status`（可选）
- `created_from` / `created_to`（可选，RFC3339）
- `deleted_from` / `deleted_to`（可选，RFC3339）
- `page`（默认 1）
- `page_size`（默认 20，最大 100）

Response：

- `items`
- `total`
- `page`
- `page_size`

说明：不直接改造既有 `GET /sandboxes`，避免影响业务侧“活跃沙箱列表”语义。

### B. 单沙箱状态流水查询

`GET /sandboxes/{id}/status-history`

Query 参数：

- `limit`（默认 50，最大 200）
- `before_id`（可选，用于下翻）

### C. 对账项检索（可选增强）

`GET /sandboxes/reconcile/items`

Query 参数：

- `sandbox_id`（可选）
- `drift_type`（可选）
- `action`（可选）
- `created_from/created_to`（可选）
- `page/page_size`

说明：若你希望本期最小变更，可先不做 C，仅做 run 维度详情查询。

## 6. 数据与索引建议

为保障查询性能，建议新增索引：

- `sandboxes(created_at DESC)`
- `sandboxes(deleted_at)`
- `sandboxes(desired_state, lifecycle_status, created_at DESC)`
- `sandbox_status_history(sandbox_id, id DESC)`（已有 `sandbox_id, created_at`，此项按分页策略决定是否需要）

## 7. 前后端改造点

后端：

- `internal/store/sandbox.go`
  - 增加元数据列表查询方法（带过滤+分页+count）
  - 增加状态流水列表查询方法
- `internal/service/sandbox.go`
  - 增加对应 service 方法与 DTO 转换
- `internal/handler/sandbox.go`
  - 注册新增查询路由
- `api/openapi.yaml`
  - 同步新增接口与 schema

前端（web）：

- 新增 API 文件或扩展 `web/src/api/sandbox.ts`
- 新增视图：
  - `web/src/views/MetadataSandboxList.vue`
  - `web/src/views/MetadataSandboxDetail.vue`
  - `web/src/views/MetadataReconcile.vue`
- 更新路由与顶部导航

## 8. 与保留策略的关系

当前保留变量为 `SANDBOX_METADATA_RETENTION_DAYS`（默认 7 天，每 1 小时清理一次）。

页面应明确提示：

- 查询结果受保留策略影响。
- 被清理数据不可在后台继续查询。

## 9. 验收标准

1. 可按状态/模板/时间筛选元数据记录，并可分页浏览。
2. 可查看任一沙箱的状态流水。
3. 可在 Web 手动触发对账并查看 run 及 item 详情。
4. OpenAPI 文档与实际行为一致。
5. 10k 量级记录下列表查询响应可接受（分页首屏 < 500ms，单机 SQLite 场景）。

## 10. 分阶段实施建议

Phase 1（最小可用）：

- 元数据列表查询 API + 页面
- 状态流水查询 API + 详情页 Tab

Phase 2：

- 对账记录中心页面（复用现有 run API）
- 手动触发对账

Phase 3（可选增强）：

- 跨 run 对账 item 检索 API
- 前端高级过滤器与保存视图

---

创建日期：2026-02-25
文档状态：待你确认
