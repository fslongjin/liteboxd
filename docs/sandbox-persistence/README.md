# Sandbox 持久化方案（面向 OpenClaw 类场景）

## 文档目的

本组文档用于评审 LiteBoxd 是否、以及如何支持以下原始需求：

1. 沙箱可长期持续运行（不因普通重启/调度变化丢状态）
2. 容器 rootfs 内写入可持久化，并在容器重启/重建后回挂
3. 可限制每个沙箱的持久化磁盘大小

## 文档清单

- `design.md`：需求拆解、现状差距、技术选型、目标架构、接口/数据模型变更、风险
- `implementation-plan.md`：分阶段实施计划、改造文件范围、测试与验收、回滚策略
- `quickstart.md`：Longhorn + 持久化模板的最短部署与验证步骤
- `verification.md`：持久化验证手册（Pod 重建验证、配额验证、Delete/Retain 行为）
- `pvc-management.md`：后端 PVC 管理能力设计（沙箱↔PVC 映射、对账与 API 方案）

## 当前结论（供你快速确认）

- 仅给 `/workspace` 挂 PVC 不满足“rootfs 全量写入持久化”，不作为目标方案。
- 目标方案采用“持久化 rootfs overlay + 每沙箱 PVC + 控制器化运行（Deployment）”。
- 磁盘限额以 PVC 容量为硬上限；`local-path` 不提供可靠硬限额，不作为生产持久化模式默认存储类。

## 已确认决策（2026-03-02）

1. `ttl=0` 表示“永久运行，不自动过期删除”。
2. 接受“仅挂载辅助容器（init/helper）提权，主业务容器不提权”。
3. `persistent-rootfs` 模式默认使用 Longhorn（或同类可硬配额 CSI）。
4. `local-path` 不作为 `persistent-rootfs` 默认存储类，仅可用于开发/测试。
