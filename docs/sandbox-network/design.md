# LiteBoxd 沙箱网络系统设计文档

## 1. 概述

本文档描述 LiteBoxd 沙箱网络系统的设计，参考 E2B 的网络功能，为沙箱提供安全的网络访问能力。

### 1.1 设计目标

1. **默认拒绝出网访问**：沙箱默认无法访问外部网络
2. **防止内部攻击**：沙箱无法访问 K3s 集群内的其他服务
3. **令牌强制认证**：所有入站访问必须通过令牌认证
4. **HTTP/WS 支持**：仅支持 HTTP/WebSocket 协议访问
5. **统一域名入口**：使用单一域名 + 路径区分不同沙箱
6. **可选域名白名单**：域名白名单功能作为后续增强

### 1.2 参考资源

- [E2B Internet Access 文档](https://e2b.dev/docs/sandbox/internet-access)
- [E2B Secured Access 文档](https://e2b.dev/docs/sandbox/secured-access)
- [Kubernetes Network Policies 文档](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [K3s Networking Services 文档](https://docs.k3s.io/networking/networking-services)
- [Cilium 官方文档](https://docs.cilium.io/)
- [Cilium K3s 安装指南](https://docs.cilium.io/en/stable/installation/k3s/)

## 2. 系统架构

### 2.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              用户 / SDK                                      │
└─────────────────────────────┬───────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         统一域名入口                                          │
│                    https://liteboxd.example.com                              │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        Ingress Controller                             │  │
│  │                        (Traefik / Nginx)                              │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────┬───────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         网关服务 (Gateway Service)                           │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  认证中间件 (X-Access-Token) │ 路由中间件 │ WebSocket 支持            │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────┬───────────────────────────────────────────────┘
                              │
                ┌─────────────┼─────────────┐
                ▼             ▼             ▼
         ┌──────────┐  ┌──────────┐  ┌──────────┐
         │ Sandbox  │  │ Sandbox  │  │ Sandbox  │  ...
         │    #1    │  │    #2    │  │    #3    │
         └──────────┘  └──────────┘  └──────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         K3s 集群网络层                                        │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                    Network Policies (默认拒绝所有)                      │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 URL 设计

```
格式: https://liteboxd.example.com/sandbox/{sandbox-id}/port/{port}
示例: https://liteboxd.example.com/sandbox/abc12345/port/3000
```

- **统一域名**：使用配置的统一域名（如 `liteboxd.example.com`）
- **路径路由**：通过路径区分沙箱 ID 和端口号
- **认证方式**：HTTP Header `X-Access-Token: {token}`

## 3. 网络隔离策略

### 3.1 CNI 选择：Cilium

**选择 Cilium 的理由**：

| 特性 | Cilium | Calico | Flannel |
|------|--------|--------|---------|
| Network Policy 支持 | ✅ 完全支持 | ✅ 完全支持 | ❌ 不支持 |
| 技术基础 | eBPF | iptables (默认) / eBPF (可选) | VXLAN |
| L7 网络策略 | ✅ 支持 | ❌ 仅 L3/L4 | ❌ |
| 可观测性 | ✅ Hubble 可视化 | ⚠️ 基础 | ❌ |
| K8s 集成 | ✅ 原生 CRD | ✅ 原生 CRD | ⚠️ 简单 |
| 性能 | ✅ 高 | ✅ 中 | ⚠️ 中 |

**Cilium 核心优势**：
1. **eBPF 数据平面**：更高性能与更强的可观测性能力
2. **L7 策略能力**：可扩展到 HTTP/gRPC 等 L7 场景
3. **Hubble 可观测性**：便于调试与故障定位
4. **CRD 持久化**：NetworkPolicy 作为 CRD 存储在 etcd 中，重启后自动恢复

### 3.1.1 远程 K3s + Cilium 部署要求

在独立机器上安装 K3s 与 Cilium，不要在 Docker 容器内安装 K3s。
参考文档：
https://docs.cilium.io/en/stable/installation/k3s/

**K3s 启动参数**：
- --flannel-backend=none
- --disable-network-policy

**安装示例**：

```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC='--flannel-backend=none --disable-network-policy' sh -
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
cilium install --version 1.18.6 --set=ipam.operator.clusterPoolIPv4PodCIDRList="10.42.0.0/16"
```

### 3.1.2 网络策略持久化保证

Cilium 的 NetworkPolicy 以 Kubernetes CRD 形式存储，具有以下特性：

1. **自动持久化**：策略创建后立即写入 K3s 的 etcd 数据库
2. **重启恢复**：K3s 重启后，Cilium 会自动从 etcd 读取并重新应用所有策略
3. **数据位置**：`/var/lib/rancher/k3s/server/db`

### 3.2 Network Policy 设计

#### 3.2.1 默认拒绝策略

为 `liteboxd` 命名空间应用默认拒绝所有入站和出站流量：

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: liteboxd
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
```

#### 3.2.2 允许 DNS 查询

允许沙箱进行 DNS 查询（需要访问 CoreDNS）：

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-dns
  namespace: liteboxd
spec:
  podSelector:
    matchLabels:
      app: liteboxd
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
```

#### 3.2.3 允许网关服务访问沙箱

允许来自网关服务的入站流量：

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-ingress
  namespace: liteboxd
spec:
  podSelector:
    matchLabels:
      app: liteboxd
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          app: liteboxd-gateway
    - podSelector:
        matchLabels:
          app: liteboxd-gateway
    ports:
    - protocol: TCP
      port: 3000-65535  # 允许所有高端口
```

#### 3.2.4 拒绝访问 K3s 内部服务

明确拒绝访问 K3s API Server 和内部服务：

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-k8s-api
  namespace: liteboxd
spec:
  podSelector:
    matchLabels:
      app: liteboxd
  policyTypes:
  - Egress
  egress:
  # 例外：允许 DNS（前面已定义）
  - to:
    - namespaceSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # 拒绝访问 kube-system 命名空间（包含 API Server）
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
  # 拒绝访问本地主机 IP（节点自身服务）
  - to:
    - ipBlock:
        cidr: 10.0.0.0/8
```

### 3.3 可选出网访问

当沙箱配置 `allowInternetAccess: true` 时，添加允许访问外部网络的策略：

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-internet-egress
  namespace: liteboxd
spec:
  podSelector:
    matchLabels:
      app: liteboxd
      internet-access: "true"  # 通过 Pod Label 标识
  policyTypes:
  - Egress
  egress:
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
        except:
        - 10.0.0.0/8     # 私有网络
        - 172.16.0.0/12
        - 192.168.0.0/16
    ports:
    - protocol: TCP
      port: 443  # 仅允许 HTTPS
    - protocol: TCP
      port: 80   # 可选：允许 HTTP
```

## 4. 网关服务设计

### 4.1 功能概述

网关服务是外部流量进入沙箱的唯一入口，负责：

1. **令牌认证**：验证 `X-Access-Token` Header
2. **路由转发**：根据路径参数将请求转发到目标沙箱
3. **协议支持**：HTTP/HTTPS/WebSocket
4. **连接管理**：处理沙箱不存在、已过期等情况

### 4.2 认证机制

#### 4.2.1 令牌生成

沙箱创建时生成随机访问令牌：

```go
// Token 格式: {sandbox-id}-{random-32-bytes}
// 存储: Pod Annotation 中
// 返回: 创建沙箱 API 响应中
```

#### 4.2.2 令牌验证流程

```
1. 客户端请求: GET /sandbox/{id}/port/{port}
   Header: X-Access-Token: {token}

2. 网关解析:
   - 从路径提取 sandbox-id
   - 从 Header 提取 token

3. 令牌验证:
   - 查询 Pod Annotation 获取存储的 token
   - 比较请求 token 与存储 token

4. 验证通过:
   - 获取 Pod IP
   - 转发请求到 {pod-ip}:{port}
```

### 4.3 API 设计

#### 4.3.1 入站访问 API

```
格式: /sandbox/{sandbox-id}/port/{port}/{path}

示例:
  GET    /sandbox/abc12345/port/3000/api/users
  POST   /sandbox/abc12345/port/8080/login
  WS     /sandbox/abc12345/port/3000/socket.io
```

#### 4.3.2 响应状态码

| 状态码 | 含义 |
|--------|------|
| 200 | 请求成功 |
| 401 | 令牌无效或缺失 |
| 404 | 沙箱不存在 |
| 410 | 沙箱已过期 |
| 502 | 沙箱服务不可用 |
| 503 | 网关内部错误 |

### 4.4 WebSocket 支持

网关需要支持 WebSocket 协议的透传转发：

1. 识别 WebSocket Upgrade 请求
2. 建立 HTTP 隧道连接到目标 Pod
3. 双向透传数据帧

补充方案与实现细节见 [websocket-proxy-plan.md](file:///home/longjin/code/lwsandbox/docs/sandbox-network/websocket-proxy-plan.md)。

## 5. 数据模型扩展

### 5.1 模板规范扩展

```go
type TemplateSpec struct {
    Image          string            `json:"image"`
    Resources      ResourceSpec      `json:"resources"`
    TTL            int               `json:"ttl"`
    Env            map[string]string `json:"env,omitempty"`
    StartupScript  string            `json:"startupScript,omitempty"`
    StartupTimeout int               `json:"startupTimeout,omitempty"`
    Files          []FileSpec        `json:"files,omitempty"`
    ReadinessProbe *ProbeSpec        `json:"readinessProbe,omitempty"`

    // 新增：网络配置
    Network        *NetworkSpec      `json:"network,omitempty"`
}

type NetworkSpec struct {
    // 是否允许访问互联网
    AllowInternetAccess bool `json:"allowInternetAccess"`

    // 可选：域名白名单（future）
    AllowedDomains []string `json:"allowedDomains,omitempty"`
}
```

### 5.2 沙箱响应扩展

```go
type Sandbox struct {
    ID              string            `json:"id"`
    Image           string            `json:"image"`
    CPU             string            `json:"cpu"`
    Memory          string            `json:"memory"`
    TTL             int               `json:"ttl"`
    Env             map[string]string `json:"env,omitempty"`
    Status          SandboxStatus     `json:"status"`
    Template        string            `json:"template,omitempty"`
    TemplateVersion int               `json:"templateVersion,omitempty"`
    CreatedAt       time.Time         `json:"created_at"`
    ExpiresAt       time.Time         `json:"expires_at"`

    // 新增：网络访问信息
    AccessToken     string            `json:"accessToken"`     // 访问令牌
    AccessURL       string            `json:"accessUrl"`       // 访问 URL 基础路径
}
```

### 5.3 创建沙箱请求扩展

```go
type CreateSandboxRequest struct {
    Template        string            `json:"template" binding:"required"`
    TemplateVersion int               `json:"templateVersion"`
    Overrides       *SandboxOverrides `json:"overrides"`

    // 新增：网络配置覆盖
    NetworkOverride *NetworkSpec      `json:"networkOverride,omitempty"`
}
```

## 6. 部署架构

### 6.1 组件部署

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              远程 K3s 集群                                   │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐     │
│  │                kube-system (Cilium DaemonSet)                      │     │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────────┐    │     │
│  │  │    Cilium      │  │    CoreDNS     │  │    API Server      │    │     │
│  │  │   (DaemonSet)  │  │                │  │                    │    │     │
│  │  └────────────────┘  └────────────────┘  └────────────────────┘    │     │
│  └────────────────────────────────────────────────────────────────────┘     │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐     │
│  │              liteboxd 命名空间                                      │     │
│  │  ┌────────────┐  ┌────────────┐  ┌──────────────────────────────┐  │     │
│  │  │   Gateway  │  │  Network   │  │         Sandboxes            │  │     │
│  │  │  Service   │  │  Policies  │  │         (Pods)               │  │     │
│  │  └────────────┘  └────────────┘  └──────────────────────────────┘  │     │
│  └────────────────────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 CNI 选择：Cilium

**最终选择：Cilium**

| 特性 | 说明 |
|------|------|
| 安装方式 | Cilium CLI |
| 数据存储 | K3s 内置 etcd (自动持久化) |
| 策略恢复 | 重启后自动从 etcd 恢复 |
| 部署位置 | `kube-system` 命名空间 |
| 数据平面 | eBPF |

**Cilium 与 K3s 集成配置**：

```bash
--flannel-backend=none
--disable-network-policy
```

```bash
cilium install --version 1.18.6 --set=ipam.operator.clusterPoolIPv4PodCIDRList="10.42.0.0/16"
```

### 6.3 远程集群接入

将远程 kubeconfig 拷贝到本机后设置环境变量：

```bash
export KUBECONFIG=~/.kube/config
```

### 6.4 网络策略持久化

**策略存储位置**：
```
/var/lib/rancher/k3s/server/db/
```

**持久化保证**：
1. NetworkPolicy CRD 自动存储在 etcd
2. Cilium DaemonSet 启动时自动读取现有策略
3. 即使删除 Cilium Pod，策略仍然存在于 etcd 中

**验证策略持久化**：
```bash
kubectl get networkpolicy -n liteboxd
```

## 7. 安全考虑

### 7.1 防止沙箱逃逸

1. **网络隔离**：Network Policy 限制沙箱只能访问指定的外部地址
2. **API Server 保护**：明确拒绝访问 `kube-system` 命名空间
3. **本地节点保护**：拒绝访问节点本地 IP 段

### 7.2 令牌安全

1. **令牌强度**：使用 32 字节随机生成的令牌
2. **令牌存储**：存储在 Pod Annotation 中（仅集群内部可访问）
3. **令牌生命周期**：与沙箱 TTL 相同，沙箱删除时令牌失效

### 7.3 访问控制

1. **强制认证**：网关服务拒绝无令牌请求
2. **速率限制**：可选的速率限制防止滥用
3. **审计日志**：记录所有访问请求用于审计

## 8. 与 E2B 功能对比

| 功能 | E2B | LiteBoxd 设计 |
|------|-----|---------------|
| 出网控制 | `allowInternetAccess` (默认 true) | ✅ 支持 (默认 false) |
| 令牌认证 | `X-Access-Token` Header (v2.0+ 强制) | ✅ 强制令牌 |
| URL 格式 | `{port}-{sandbox-id}.e2b.app` | `domain/sandbox/{id}/port/{port}` |
| TCP 支持 | 不详 | ❌ 仅 HTTP/WS |
| 域名白名单 | 不详 | ⚠️ 可选增强 |
| 网络策略 | 未公开 | ✅ Network Policy |

## 9. 未来扩展

### 9.1 域名白名单

允许配置允许访问的外部域名列表：

```go
type NetworkSpec struct {
    AllowInternetAccess bool     `json:"allowInternetAccess"`
    AllowedDomains      []string `json:"allowedDomains,omitempty"`
}
```

实现方式：
- 在网关服务层面实现 HTTP 代理
- 或使用 Istio/Linkerd 等服务网格

### 9.2 TCP 端口访问

如需支持 TCP 端口访问，可考虑：
- 使用 SNI Proxy（需要 TLS）
- 使用 SOCKS 代理协议
- 或独立的 TCP 网关服务

### 9.3 多租户支持

通过命名空间隔离实现多租户：
- 每个租户独立的命名空间
- 跨命名空间 Network Policy 隔离
