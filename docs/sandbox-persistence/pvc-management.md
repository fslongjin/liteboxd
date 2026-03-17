# LiteBoxd 后端 PVC 管理能力设计

## 1. 背景

当前持久化沙箱已经有 PVC 生命周期基础能力，但对“运营可见性”仍不足。  
你关心的核心是：

1. 能看到当前有哪些 PVC。
2. 能看到每个 PVC 对应哪个 sandbox。
3. 能识别异常关系（孤儿 PVC、元数据缺失、PVC 丢失）。

## 2. 当前实现现状（as-is）

已具备：

1. 创建持久化 sandbox 时会记录：
   - `volume_claim_name`
   - `storage_class_name`
   - `persistence_size`
   - `volume_reclaim_policy`
2. 删除 sandbox 时按 `reclaimPolicy` 处理 PVC：
   - `Delete` 删除 PVC
   - `Retain` 保留 PVC
3. `GET /sandboxes` 与 `GET /sandboxes/metadata` 已返回 `persistence` 字段（含 `volumeClaimName`）。

缺口：

1. 缺少专门的“PVC 视图接口”（仅看 sandbox 不够直观）。
2. 缺少 DB 与 K8s PVC 的对账状态。
3. 缺少 orphan/dangling 分类，排障成本高。

## 2.1 现在就能用的临时查看方法（无代码改动）

当前可先用 `GET /sandboxes/metadata` 读取 `persistence.volumeClaimName`：

```bash
curl -s http://<liteboxd-host>/api/v1/sandboxes/metadata?page=1&page_size=100 \
  | jq -r '.items[] | [.id, .template, (.persistence.volumeClaimName // "-"), (.persistence.storageClassName // "-"), .lifecycle_status] | @tsv'
```

它能解决“sandbox -> pvc”查询，但还不支持“pvc -> sandbox”反向查、孤儿识别和统一对账视图。

## 3. 目标能力（to-be）

新增后端 PVC 管理视图，支持：

1. 按 PVC 维度列出所有记录。
2. 给出 sandbox 映射关系。
3. 给出对账状态：
   - `bound`：DB 与 K8s 都存在且一致
   - `orphan_pvc`：K8s 有 PVC，DB 无对应 active/deleted 记录
   - `deleting`：DB 记录仍在删除中，PVC 已按预期删掉
   - `dangling_metadata`：DB 有记录，K8s 无 PVC
4. 支持按命名空间、storageClass、sandboxId、状态筛选。

## 4. API 设计建议

## 4.1 列表接口

`GET /api/v1/sandboxes/pvcs`

查询参数：

- `sandbox_id`（可选）
- `storage_class`（可选）
- `state`（可选，`bound|orphan_pvc|deleting|dangling_metadata`）
- `page` / `page_size`

返回示例（核心字段）：

```json
{
  "items": [
    {
      "pvcName": "sandbox-data-abc123",
      "namespace": "liteboxd-sandbox",
      "storageClassName": "longhorn",
      "requestedSize": "1Gi",
      "phase": "Bound",
      "pvName": "pvc-xxxx",
      "sandboxId": "abc123",
      "sandboxLifecycleStatus": "running",
      "reclaimPolicy": "Delete",
      "state": "bound",
      "source": "db+k8s"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

## 4.2 单 PVC 详情

`GET /api/v1/sandboxes/pvcs/{pvcName}`

返回：

1. DB 记录信息（若存在）
2. K8s PVC 实时信息（若存在）
3. 归类状态与诊断建议

## 5. 服务层实现建议

在 `SandboxService` 新增 `ListPVCMappings(ctx, opts)`：

1. 从 DB 读取 `volume_claim_name != ''` 的 sandbox 元数据。
2. 从 K8s 拉取 sandbox namespace 下 PVC 列表（可按 `app=liteboxd` label 限定）。
3. 以 `pvcName` 做 full-join，生成统一视图项。
4. 按规则打 `state`：
   - DB + K8s 都有：`bound`
   - 仅 K8s：`orphan_pvc`
   - 仅 DB 且 sandbox 正在 `terminating`：`deleting`
   - 仅 DB 且不在删除中：`dangling_metadata`

## 6. 数据模型建议

不必先建新表，第一版可直接复用现有 `sandboxes` 表字段：

- `id`
- `volume_claim_name`
- `storage_class_name`
- `persistence_size`
- `volume_reclaim_policy`
- `lifecycle_status`

后续若要做审计/历史，可再加 `sandbox_pvc_events` 表（可选）。

## 7. 与现有 reconcile 的关系

建议把 PVC 对账纳入 `sandbox_reconcile`：

1. 新增 drift 类型：
   - `pvc_orphan`
   - `pvc_missing`
2. `action` 默认 `alert_only`，避免自动删错数据。
3. 手动确认后再做清理动作。

## 8. 最小实施路径（建议）

Phase 1（快速可用）：

1. 新增 `GET /sandboxes/pvcs` 只读接口。
2. 返回 sandbox ↔ pvc 映射 + 对账状态。
3. 前端新增 PVC 管理页（列表 + 过滤）。

Phase 2（排障增强）：

1. 新增 `GET /sandboxes/pvcs/{name}` 详情。
2. reconcile 纳入 PVC drift 告警。

Phase 3（治理能力）：

1. 审批式清理 orphan PVC（非自动）。
2. 容量统计、增长趋势、告警阈值。

## 9. 验收标准

1. 可以一眼看出“每个 sandbox 对应哪个 PVC”。
2. 可以识别 orphan/dangling 情况。
3. 删除 sandbox 后 PVC 行为与 `reclaimPolicy` 一致并可审计。
4. API 与 OpenAPI 文档保持一致。
