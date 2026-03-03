# Sandbox 持久化实施计划（分阶段）

## 1. 实施目标

在不破坏现有非持久化沙箱功能的前提下，增量交付 rootfs 持久化模式，并满足：

1. 持久运行
2. rootfs 全量写入持久化
3. 单沙箱磁盘上限

## 2. 交付策略

- 双模式并存：
  - 现有模式：`ephemeral`（当前默认）
  - 新模式：`persistent-rootfs`（模板显式开启）
- 先 PoC，后主干；先 API/模型，再 runtime；最后联调与回归。
- `persistent-rootfs` 默认存储类基线：Longhorn（单机可用，副本建议设为 1；多节点按可用性策略配置）。

## 3. Phase 0：PoC 与可行性闸门

目标：验证 rootfs overlay 技术路线在目标集群可运行。

任务：

1. 在独立分支实现最小 PoC（单模板、单沙箱）
2. 验证容器内 overlay/fuse 挂载能力
3. 验证 rootfs 写入在 Pod 重建后可恢复
4. 验证 PVC 容量超限行为
5. 验证 Longhorn 单节点（副本=1）与多节点配置下行为一致性

通过标准（全部满足才进入 Phase 1）：

- `/root` 与 `/opt` 写入可恢复
- Pod 删除后自动恢复并回挂原状态
- 配额超限能稳定复现写失败

## 4. Phase 1：API / Model / OpenAPI / DB

目标：先把“声明面”定义完整并可落库。

任务：

1. 更新 `backend/internal/model/template.go`
- 新增 `PersistenceSpec`

2. 更新 `backend/internal/model/sandbox.go`
- 新增 overrides.persistence
- `ttl` 支持 `0`（若评审通过）

3. 更新 `api/openapi.yaml`
- 增加 persistence schema
- 调整 ttl 约束与示例

4. 更新 `backend/internal/store/sqlite.go` 与 `store/sandbox.go`
- 增加持久化相关字段
- 补充查询/扫描逻辑

5. 补充模型与存储单测

完成定义（DoD）：

- API 入参、模型、落库字段一致
- OpenAPI 与 handler 行为一致

## 5. Phase 2：K8s Runtime 改造（核心）

目标：从 Pod 直建演进到 Deployment + PVC。

任务：

1. 扩展 `backend/internal/k8s/client.go`
- PVC create/get/delete
- Deployment create/get/delete
- 按 label 查当前活跃 Pod

2. 改造 `backend/internal/service/sandbox.go`
- 创建：`CreateVolume -> CreateDeployment`
- 删除：按 reclaimPolicy 决定 PVC 回收
- 状态更新改为基于 Deployment/Pod 观测

3. 更新 sandbox RBAC（`deploy/sandbox/rbac-api.yaml`）
- 增加 `persistentvolumeclaims`、`deployments` 必要权限

4. 持久模式下接入 rootfs overlay 启动流程

完成定义（DoD）：

- 持久模式 sandbox 可创建、可运行、可删除
- Pod 被删后自动恢复
- 非持久模式行为不回归

## 6. Phase 3：Gateway / Exec / Files / Reconcile 适配

目标：打通现有访问与运维路径。

任务：

1. `backend/internal/gateway/proxy.go`
- 目标 Pod 解析改为 label 查询，不依赖固定 Pod 名

2. `backend/internal/k8s/client.go` exec/upload/download
- 路径与命令对齐 merged rootfs 语义

3. `backend/internal/service/sandbox_reconcile.go`
- 漂移项纳入 Deployment/PVC
- 支持“active + persistent”自动修复

4. 增加状态历史事件字段，标识恢复来源（watcher/reconciler）

完成定义（DoD）：

- Gateway 代理在 Pod 重建后仍可访问
- exec/file 操作与用户视角 rootfs 一致
- reconcile 能发现并修复关键漂移

## 7. Phase 4：观测、压测、文档与发布

目标：可上线、可回滚、可运维。

任务：

1. 指标与告警
- PVC 使用率
- rootfs 挂载失败
- 自动恢复次数

2. E2E 测试集
- `test_persistent_rootfs_write_recover`
- `test_persistent_disk_quota`
- `test_pod_recreate_recover`
- `test_ttl_zero_no_auto_delete`

3. 文档
- 更新 `docs/user/deploy.md`：存储类前置条件
- 更新模板示例（新增 persistence 字段）

4. 灰度上线
- 先对单模板灰度
- 再扩大到 code-interpreter 模板

完成定义（DoD）：

- 核心 E2E 全通过
- 监控面板可见关键指标
- 发布说明包含回滚手册

## 8. 里程碑与风险控制

里程碑建议：

1. M1（Phase 0-1）: 声明面 + PoC 可行
2. M2（Phase 2）: 持久模式可创建/可恢复
3. M3（Phase 3-4）: 全链路可用并可灰度

关键风险与缓解：

1. 挂载能力不稳定
- 缓解：Phase 0 设置硬闸门，不通过不进主线

2. 存储类不满足硬配额
- 缓解：创建时强校验并拒绝不合格存储类

3. 权限提升带来安全风险
- 缓解：专用节点池 + 最小权限 + 独立审计

## 9. 回滚策略

- 功能级开关：`persistence.enabled=false` 回退到现有模式
- 控制面回滚：保留旧版 API/Service 镜像，按 Deployment 回滚
- 数据面回滚：保留已创建 PVC（即使功能回退，数据不删）

## 10. 最终验收清单（面向你的 3 条需求）

- [ ] 沙箱 crash / Pod 删除后自动恢复运行
- [ ] `/root`、`/opt`、`/var/lib` 写入在重建后保留
- [ ] 每沙箱可配置容量并在超限时写失败
- [ ] OpenAPI、后端行为、文档三者一致
- [ ] 非持久化模式无回归
