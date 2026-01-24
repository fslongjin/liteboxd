# 沙箱模版系统文档

## 概述

本目录包含 LiteBoxd 轻量级沙箱模版系统的完整设计文档。该系统参考 [E2B](https://e2b.dev/docs/sandbox-template) 的设计理念，为 LiteBoxd 提供模版化的沙箱管理能力。

## 文档目录

| 文档 | 说明 |
|------|------|
| [design.md](./design.md) | 系统总体设计方案，包括核心概念、架构设计、实现计划 |
| [api-spec.md](./api-spec.md) | 完整的 API 规范，包括请求/响应格式、错误码 |
| [database-design.md](./database-design.md) | 数据库表结构设计、Go 数据模型、查询示例 |

## 快速概览

### 核心功能

1. **模版定义**: 通过 API 创建可复用的沙箱配置模版
2. **版本管理**: 每次更新自动创建新版本，支持回滚
3. **快速实例化**: 从模版一键创建沙箱，支持参数覆盖
4. **启动脚本**: 支持沙箱启动后自动执行初始化脚本
5. **文件预置**: 支持创建沙箱时自动上传预设文件
6. **镜像预拉取**: 提前将镜像拉取到节点，加速沙箱创建
7. **YAML 导入**: 支持通过 YAML 文件批量导入模版

### 技术决策

| 决策项 | 选择 | 说明 |
|--------|------|------|
| 数据存储 | **SQLite** | 轻量级，无需额外部署 |
| 多租户 | **暂不支持** | 保持简单，后续可扩展 |
| 镜像预拉取 | **支持** | 通过 K8s DaemonSet 实现 |
| YAML 导入 | **支持** | 支持单个和批量导入 |
| Dockerfile 构建 | **不支持** | 直接使用现有镜像 |

### 与 E2B 的主要区别

| 特性 | E2B | LiteBoxd |
|------|-----|----------|
| 模版定义 | Dockerfile + 构建 | YAML/JSON 配置 (无构建) |
| 底层技术 | Firecracker microVM | Kubernetes Pod |
| 隔离级别 | 硬件级 (KVM) | 容器级 (cgroups) |

## 实现计划

### Phase 1: 核心模版功能 (优先)
- SQLite 数据库初始化
- 模版 CRUD API
- 从模版创建沙箱

### Phase 2: 镜像预拉取
- 预拉取服务和 API
- DaemonSet 管理
- 自动预拉取选项

### Phase 3: YAML 导入导出
- YAML 解析和验证
- 导入/导出 API
- 三种导入策略

### Phase 4: 高级功能
- 启动脚本执行
- 文件预置上传
- 就绪探针检查

### Phase 5: 前端界面
- 模版管理界面
- YAML 导入界面
- 预拉取状态展示

### Phase 6: 内置模版
- base, python, nodejs, golang 等常用模版

---

**创建日期**: 2025-01-24
**参考**: [E2B Sandbox Template Documentation](https://e2b.dev/docs/sandbox-template)
