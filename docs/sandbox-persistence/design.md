# LiteBoxd 沙箱持久化设计（rootfs 全量持久化）

## 1. 背景与目标

你提出的目标是让 LiteBoxd 能运行类似 OpenClaw 的“可长期存在、可恢复状态”的沙箱，而不是一次性临时 Pod。

本设计以“满足原始需求”为唯一目标，不采用仅覆盖部分目录的折中方案。

## 已确认决策（2026-03-02）

1. 支持 `ttl=0`，语义为“永久运行，不自动过期删除”。  
2. rootfs 持久化模式采用“仅挂载辅助容器提权，主业务容器不提权”。  
3. `persistent-rootfs` 默认使用 Longhorn（或同类支持硬配额的 CSI）；`local-path` 不作为默认选项。  

## 2. 原始需求转化为验收标准

### 2.1 持久运行

- 沙箱进程异常退出后，平台自动拉起。
- 节点重启/Pod 重建后，沙箱可自动恢复运行。
- 可配置不自动 TTL 删除（建议 `ttl=0` 表示永久，手动删除）。

### 2.2 rootfs 全量写入持久化

- 用户在 `/` 下任意可写路径产生的数据（如 `/root`、`/opt`、`/var/lib/...`）在容器重启/重建后仍在。
- 平台重建工作负载时可回挂同一持久化层，而不是重新从镜像“空白启动”。

### 2.3 单沙箱持久化磁盘上限

- 每个沙箱有独立持久卷容量上限。
- 超限后写入失败，且不会侵占其他沙箱配额。

## 3. 当前实现与差距

基于当前代码（截至本次评审）观察到：

1. `backend/internal/k8s/client.go`
- `/workspace` 使用 `EmptyDir`，非持久。
- 创建的是单 Pod（非控制器），并且 `RestartPolicy` 当前为 `Never`。

2. `backend/internal/service/sandbox.go`
- 生命周期围绕“创建 Pod / 删除 Pod”，无 PVC 生命周期管理。
- TTL cleaner 到期会直接删除沙箱。

3. `api/openapi.yaml` 与 model
- `TemplateSpec` / `CreateSandboxRequest` 无 persistence 字段。
- 无 per-sandbox 持久化容量字段与回收策略字段。

结论：当前实现不满足你的 3 条原始需求。

## 4. 方案对比

### 4.1 方案 A：仅 `/workspace` 挂 PVC（不选）

优点：实现快、改动小。

缺点：
- 不满足“rootfs 全量写入持久化”。
- 用户写入 `/root`、`/var` 等仍会丢失。

结论：不满足原始需求，淘汰。

### 4.2 方案 B：持久化 rootfs overlay（推荐）

核心思想：

- 每个沙箱创建独立 PVC。
- 启动时将镜像 rootfs 作为基线层（lower/base），将可写层（upper/work）落到 PVC。
- 实际业务进程在合成后的 merged rootfs 中运行。
- Pod/容器重建后继续使用同一 PVC 的 upper/work，因此 rootfs 写入可恢复。

优点：
- 直接满足 rootfs 全量持久化。
- 与“容器重启/重建回挂”目标一致。

代价：
- 运行时复杂度和权限要求提高（overlay/fuse mount 能力）。
- exec/file API 需适配 merged rootfs 语义。

## 5. 目标架构

## 5.1 控制面资源模型

每个 sandbox 对应：

1. `PersistentVolumeClaim`：`sandbox-data-{id}`
2. `Deployment`（1 副本）：`sandbox-{id}`
3. （可选）`Service`：按需暴露固定 selector

为什么从“直接 Pod”改为“Deployment”：

- 需要自愈（容器 crash 自动重建）。
- 需要在节点异常后由控制器重建 Pod。
- 与持久卷复挂场景更匹配。

## 5.2 rootfs 持久化机制

容器启动分为两步：

1. `init` 阶段
- 仅 `init/helper` 容器提权，在 PVC 上执行 overlay 挂载准备。
- `lowerdir` 直接使用镜像 rootfs（只读），`upper/work` 落在 PVC。
- 不再做“整份 rootfs 拷贝到 PVC”，避免启动慢和小盘初始化失败。

2. `runtime` 阶段
- 挂载 `overlay(base, upper, work) -> merged`（upper/work 在 PVC 上）。
- 业务进程在 merged rootfs 中运行。

说明：
- 这保证“rootfs 写入”进入 PVC 的 upper 层。
- Pod 重建后 upper/work 不变，状态可恢复。

## 5.3 持久化与 TTL 语义

- `ttl > 0`：保持现有自动过期删除语义。
- `ttl = 0`：不自动过期，只能手动删除。
- 删除时按 `reclaimPolicy` 处理 PVC：
  - `Delete`：删除 PVC（默认）
  - `Retain`：保留 PVC，便于人工恢复/审计

## 5.4 磁盘大小限制

- 每沙箱 PVC 请求容量 `size` 即磁盘上限。
- 沙箱创建前校验 StorageClass 能力。
- 对不具备硬配额保障的存储类（典型 `local-path`）默认拒绝 rootfs 持久化模式创建。
- 默认推荐 StorageClass：`longhorn`。单节点场景可用（副本数设为 1），多节点场景建议按可用性策略设置副本数。

注：`local-path` 的容量不具备可靠硬限制（见 rancher/local-path-provisioner issue #190 中“does not enforce capacity limitations”）。

## 6. API 与模板模型变更

## 6.1 TemplateSpec 新增

```yaml
spec:
  persistence:
    enabled: true
    mode: rootfs-overlay    # 当前只支持该模式
    size: 1Gi
    storageClassName: longhorn
    reclaimPolicy: Delete   # Delete | Retain
```

## 6.2 CreateSandboxRequest.overrides 新增

```json
{
  "overrides": {
    "ttl": 0,
    "persistence": {
      "size": "40Gi"
    }
  }
}
```

约束：

- `mode` 不允许 override。
- `size` 允许在模板上限策略内 override（可选配置：禁止放大，只允许缩小）。

## 6.3 OpenAPI 同步

必须同步更新 `api/openapi.yaml`：

- `TemplateSpec.persistence`
- `CreateSandboxRequest.overrides.persistence`
- `ttl` 最小值从 `1` 调整为 `0`（若采纳永久语义）

## 7. 数据模型与存储变更

`sandboxes` 表新增字段（建议）：

- `persistence_enabled` BOOLEAN
- `persistence_mode` TEXT
- `persistence_size` TEXT
- `storage_class_name` TEXT
- `volume_claim_name` TEXT
- `volume_reclaim_policy` TEXT
- `runtime_kind` TEXT（`pod` / `deployment`）
- `runtime_name` TEXT

用途：

- 支持重建、排障、回收与审计。
- 让 reconcile 能检查“DB 声明状态 vs K8s Deployment/PVC 实际状态”。

## 8. 服务层与网关改造点

1. `k8s.Client`
- 新增：Create/Delete/Get Deployment、Create/Delete/Get PVC、按 label 查 Pod。

2. `SandboxService`
- 创建流程从 `CreatePod` 改为 `CreateVolume + CreateDeployment`。
- 删除流程根据 reclaimPolicy 决定是否删 PVC。

3. `Gateway`
- 不能再依赖固定 Pod 名 `sandbox-{id}`。
- 改为按 `sandbox-id` label 动态解析当前活跃 Pod。

4. `Exec/File`
- 命令执行与文件路径需对齐 merged rootfs（避免误操作到临时层）。

5. `Reconcile`
- 漂移检查对象扩展到 Deployment/PVC。
- 发现 Deployment 丢失时可按 metadata 自动重建（active + persistence 模式）。

## 9. 安全与隔离

rootfs overlay 模式比当前模式更敏感，需增加边界：

- 只给 `init/helper` 容器提权，主业务容器保持非特权。
- 将持久化沙箱调度到专用 node pool（污点/容忍）。
- 单独 runtimeClass / PSP(或等价策略) 控制权限。
- 强制 seccomp、drop capabilities（除挂载必须项）。
- 对 `/proc`、`/sys`、宿主路径保持最小暴露。

## 10. 可观测性

新增指标与事件：

- `sandbox_persistence_pvc_usage_bytes`
- `sandbox_rootfs_mount_fail_total`
- `sandbox_recover_success_total`
- `sandbox_disk_quota_exceeded_total`

日志中增加：

- `sandbox_id`、`pvc`、`storage_class`、`reclaim_policy`、`runtime_name`

## 11. 验收测试（对应原始需求）

1. 持久运行
- 杀掉主进程，Deployment 自动恢复。
- 删除 Pod 后自动拉起且业务恢复。

2. rootfs 持久化
- 在 `/root/.cache`、`/opt/app-data` 写入文件。
- 重启/重建后文件仍存在。

3. 磁盘限制
- `dd` 持续写入直到超过 `size`。
- 写入应失败，并产生可观测告警。

## 12. 风险与边界

1. 存储类能力差异
- 若底层存储不支持硬配额，需求 3 无法强保证。

2. 权限模型变更
- overlay/fuse 挂载能力会提高攻击面，需专用节点和严格策略。

3. 性能开销
- 首次 rootfs 初始化、overlay 层写入会带来延迟。

4. 镜像兼容性
- 极少数镜像对 entrypoint/初始化流程敏感，需要模板侧适配。

## 13. 当前评审结论

- 方案 B（持久化 rootfs overlay）作为满足需求的目标方案：`同意`
- `ttl=0` 语义：`同意`
- rootfs 持久化模式默认不使用 `local-path`，默认使用 Longhorn（或同类 CSI）：`同意`
- 先做 PoC 再推进全量改造：`同意（按实施计划执行）`
