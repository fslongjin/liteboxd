# LiteBoxd 持久化沙箱资源删除与强制清理设计

## 背景与问题

当前带持久卷的沙箱删除存在一个明显的控制面与数据面脱节问题：

1. Web 控制台删除后，主列表很快不再显示该沙箱。
2. Kubernetes 中的 Deployment、Pod、PVC 可能仍然存在。
3. 在 `reclaimPolicy=Delete` 场景下，PVC 甚至可能长期卡住不被回收。

这意味着 LiteBoxd 当前把“删除请求已接受”误报成了“删除已完成”。对于 `reclaimPolicy=Delete`，平台必须对最终回收负责，不能把卡住的清理由用户手工兜底。

## 根因分析

根因不是单点故障，而是删除流程建模错误：

1. `DELETE` API 提前 `MarkDeleted`
   - 当前删除入口在发出 K8s 删除请求后就立即把 DB 记录标记为 `deleted`。

2. 主列表只看 active
   - Web 主列表基于 active metadata 展示，因此一旦 `desired_state=deleted`，UI 就会立即消失。

3. 删除流程不是状态机
   - 当前只有“调用一次 Delete”，没有阶段、超时、重试、升级强删等机制。

4. 重启恢复缺少删除阶段持久化
   - Server 重启后虽然能重新 reconcile，但无法知道上次删除进行到了哪一步、是否已经进入强删、下次何时重试。

## 目标与非目标

### 目标

1. 删除必须最终收敛。
2. `reclaimPolicy=Delete` 必须具备分级强删能力。
3. 删除过程必须可恢复，server 在任意阶段重启都能继续。
4. 删除过程必须可观测，metadata 和 detail 能看见当前删除阶段。

### 非目标

1. 不做 PVC 数据恢复能力。
2. 不做多副本 server 的 leader election。
3. 不做跨 namespace 的通用存储治理平台。

## 删除状态机

LiteBoxd 为每个 sandbox 持久化删除阶段：

1. `requested`
2. `quiescing_runtime`
3. `deleting_storage`
4. `force_cleanup`
5. `verifying`
6. `completed`

语义约束：

1. `DELETE /sandboxes/{id}` 只会把删除请求落库到 `requested`。
2. 只有删除执行器能推进阶段。
3. 只有 `completed` 才允许把 `lifecycle_status` 置为 `deleted`。

## 分级强删策略

### Level 0 常规删除

1. 删除 Deployment 或 Pod。
2. `Delete` 策略下删除 PVC。
3. 等待自然收敛。

### Level 1 Runtime 强删

1. 强制删除 Pod。
2. Deployment 卡住时移除其 finalizer。

### Level 2 PVC 强删

1. 确认 runtime 已不再引用 PVC。
2. 移除 PVC finalizer。
3. 再次发起 PVC 删除。

### Level 3 PV / VolumeAttachment 清理

1. 确认 PV 归属该 PVC。
2. 删除相关 VolumeAttachment。
3. 必要时移除 PV finalizer 并删除 PV。

## 重启恢复机制

Server 启动时会扫描：

1. `desired_state=deleted`
2. `lifecycle_status!=deleted`
3. `deletion_phase!=completed`

这些记录全部进入删除执行器继续推进。执行器完全以 DB 状态和 K8s 当前快照为依据，不依赖内存态。

## 数据模型变更

`sandboxes` 表新增字段：

1. `deletion_phase`
2. `deletion_started_at`
3. `deletion_last_attempt_at`
4. `deletion_next_retry_at`
5. `deletion_attempts`
6. `deletion_force_level`
7. `deletion_last_error`

默认语义：

1. 新建 sandbox 这些字段为空或零值。
2. 删除请求进入后开始填充。
3. 只有 `deletion_phase=completed` 才允许被 metadata cleaner 清理。

## API / OpenAPI 变更

1. `DELETE /sandboxes/{id}` 改为 `202 Accepted`。
2. 返回体包含当前 sandbox metadata 和 `deletion` 字段。
3. `Sandbox` / metadata 响应增加：
   - `deletion.phase`
   - `deletion.startedAt`
   - `deletion.lastAttemptAt`
   - `deletion.nextRetryAt`
   - `deletion.attempts`
   - `deletion.forceLevel`
   - `deletion.lastError`

## 实施步骤

1. 先补 `store/sqlite/model`，把删除阶段持久化下来。
2. 新增 `SandboxDeletionService`，统一推进删除状态机。
3. 改造 API 删除和 TTL cleaner，只发起删除请求，不直接完成删除。
4. 收缩 reconcile 对 deleted sandbox 的职责，避免多处并发判定删除完成。
5. 补 Web 展示和返回语义。
6. 补测试、构建、验证文档。

## 测试与验收

需要覆盖：

1. 删除请求不会立即把 sandbox 标记成 `deleted`。
2. `reclaimPolicy=Delete` 正常情况下能最终删除 PVC。
3. `reclaimPolicy=Retain` 不误删 PVC。
4. Pod、PVC、PV/VolumeAttachment 卡住时会升级到对应强删等级。
5. server 在删除进行中重启后仍会继续删除。

手工验证重点：

1. Web 删除后主列表可消失，但 metadata/detail 应显示删除阶段。
2. K8s 中残留资源最终会收敛。
3. metadata cleaner 不会提前清掉未完成删除的 DB 记录。

## 风险与反思点

1. finalizer 强删必须基于资源归属校验，不能模糊匹配。
2. metadata cleaner 不能清理 `deletion_phase!=completed` 的记录。
3. 控制台不再把“删除请求成功”误导成“删除已完成”。

## 实施后反思

当前实现已经把删除职责收口到单一删除执行器，并把状态持久化到 DB。后续如果发现以下问题，需要继续修正：

1. 删除强删阈值是否需要做成配置项。
2. 是否需要在 metadata 页面增加 `deletion_phase` 过滤。
3. 是否需要单独暴露删除任务对账接口，便于运营排障。
