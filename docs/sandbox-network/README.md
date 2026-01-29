# LiteBoxd 沙箱网络系统设计

本目录包含 LiteBoxd 沙箱网络系统的完整设计和实现计划。

## 文档导航

| 文档 | 描述 |
|------|------|
| [design.md](./design.md) | 完整的系统设计文档，包含架构、网络策略、安全考虑 |
| [implementation-plan.md](./implementation-plan.md) | 分阶段实现计划，包含任务清单和验收标准 |
| [network-policies/](./network-policies/) | Kubernetes Network Policy 资源示例 |

## 快速概览

### 设计目标

1. **默认拒绝出网访问**：沙箱默认无法访问外部网络
2. **防止内部攻击**：沙箱无法访问 K3s 集群内的其他服务
3. **令牌强制认证**：所有入站访问必须通过令牌认证
4. **HTTP/WS 支持**：仅支持 HTTP/WebSocket 协议访问
5. **统一域名入口**：使用单一域名 + 路径区分不同沙箱
6. **可选域名白名单**：域名白名单功能作为后续增强

### URL 设计

```
格式: https://liteboxd.example.com/sandbox/{sandbox-id}/port/{port}
示例: https://liteboxd.example.com/sandbox/abc12345/port/3000
```

### 认证方式

```http
GET /sandbox/abc12345/port/3000/api/users
X-Access-Token: {sandbox-access-token}
```

### 架构图

```
用户/SDK
    │
    ▼
统一域名入口 (https://liteboxd.example.com)
    │
    ▼
Ingress Controller (Traefik/Nginx)
    │
    ▼
网关服务 (令牌认证 + 路由)
    │
    ├──► Sandbox #1
    ├──► Sandbox #2
    └──► Sandbox #3
```

## 与 E2B 对比

| 功能 | E2B | LiteBoxd |
|------|-----|----------|
| 出网控制 | `allowInternetAccess` (默认 true) | ✅ 支持 (默认 **false**) |
| 令牌认证 | `X-Access-Token` (v2.0+ 强制) | ✅ 强制令牌 |
| URL 格式 | `{port}-{id}.e2b.app` | `domain/sandbox/{id}/port/{port}` |
| 网络策略 | 未公开 | ✅ Cilium Network Policy |
| TCP 支持 | 不详 | ❌ 仅 HTTP/WS |
| 域名白名单 | 不详 | ⚠️ 可选增强 |

## CNI 选择：Cilium

### 为什么选择 Cilium？

| 特性 | Cilium | Calico | Flannel |
|------|--------|--------|---------|
| Network Policy | ✅ | ✅ | ❌ |
| 技术基础 | **eBPF** | iptables | VXLAN |
| L7 策略 | ✅ HTTP/gRPC | ❌ | ❌ |
| 可观测性 | ✅ Hubble | ⚠️ | ❌ |
| 性能 | **最高** | 中 | 中 |

### 远程集群要求

在独立机器上部署 K3s 与 Cilium，不要在 Docker 容器内安装 K3s。
参考文档：
https://docs.cilium.io/en/stable/installation/k3s/

将远程 kubeconfig 拷贝到本机后设置：

```bash
export KUBECONFIG=~/.kube/config
```

### Phase 1: Cilium 网络策略基础
- 准备远程 K3s + Cilium 集群
- 定义基础 Network Policy 资源
- 验证策略持久化

### Phase 2: 数据模型与 API 扩展
- 扩展 Template 和 Sandbox 模型
- 添加网络配置字段
- 实现访问令牌生成

### Phase 3: 网关服务实现
- 实现认证中间件
- 实现 HTTP/WS 代理转发
- 部署网关服务

### Phase 4: 集成测试与文档
- 端到端测试
- 文档和示例代码

### Phase 5+: 可选增强
- 域名白名单
- TCP 端口访问
- 速率限制与审计

## 关键技术决策

### 网络隔离策略

```yaml
# 1. 默认拒绝所有
policyTypes: [Ingress, Egress]

# 2. 允许 DNS
to: [kube-system/kube-dns]

# 3. 允许网关入站
from: [liteboxd-gateway]

# 4. 拒绝 K8s API 访问
# 通过 default-deny 实现隐式拒绝

# 5. 可选互联网访问
to: [0.0.0.0/0]  // 仅当 internet-access=true
```

### K3s 启动参数

```bash
--disable=traefik
--flannel-backend=none
--disable-network-policy
```

## 参考资源

- [E2B Internet Access](https://e2b.dev/docs/sandbox/internet-access)
- [E2B Secured Access](https://e2b.dev/docs/sandbox/secured-access)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Cilium 官方文档](https://docs.cilium.io/)
- [Cilium K3s 安装指南](https://docs.cilium.io/en/stable/installation/k3s/)
- [K3s Networking](https://docs.k3s.io/networking/networking-services)

## 变更历史

| 日期 | 版本 | 变更说明 |
|------|------|----------|
| 2025-01-24 | 1.0 | 初始设计版本 |
| 2025-01-24 | 1.1 | 选择 Cilium 作为 CNI，调整为远程集群部署 |
