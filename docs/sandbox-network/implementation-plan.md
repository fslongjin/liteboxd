# LiteBoxd 沙箱网络系统实现计划

## 阶段概览

| 阶段 | 名称 | 预计复杂度 | 依赖 |
|------|------|-----------|------|
| Phase 1 | CNI 网络策略基础 | 中 | 无 |
| Phase 2 | 数据模型与 API 扩展 | 低 | Phase 1 |
| Phase 3 | 网关服务实现 | 高 | Phase 1, 2 |
| Phase 4 | 集成测试与文档 | 中 | Phase 1, 2, 3 |
| Phase 5+ | 可选增强功能 | 高 | Phase 1-4 |

---

## Phase 1: CNI 网络策略基础

### 目标
使用远程 K3s 集群与 Cilium，完成网络策略基础能力。

### 任务清单

#### 1.1 远程 K3s + Cilium 集群准备
- [ ] 在独立机器上安装 K3s（禁用 Flannel 与内置策略控制器）
- [ ] 安装 Cilium 并验证状态
- [ ] 将 kubeconfig 拷贝到本机并配置 KUBECONFIG

**Cilium 选择理由**：
- eBPF 技术提供更高性能
- 完整的 L7 网络策略支持
- Hubble 提供网络可观测性
- NetworkPolicy 自动持久化到 etcd

**实现细节：**

```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC='--flannel-backend=none --disable-network-policy' sh -
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
cilium install --version 1.18.6 --set=ipam.operator.clusterPoolIPv4PodCIDRList="10.42.0.0/16"
cilium status --wait
```

#### 1.2 Network Policy 资源定义
- [ ] 创建 `deploy/network-policies/` 目录
- [ ] 定义默认拒绝策略 (`default-deny-all.yaml`)
- [ ] 定义 DNS 允许策略 (`allow-dns.yaml`)
- [ ] 定义网关入站策略 (`allow-gateway-ingress.yaml`)
- [ ] 定义 K8s API 拒绝策略 (`deny-k8s-api.yaml`)

**文件结构：**
```
deploy/network-policies/
├── base/
│   ├── default-deny-all.yaml
│   ├── allow-dns.yaml
│   ├── deny-k8s-api.yaml
│   └── allow-gateway-ingress.yaml
└── internet/
    └── allow-internet-egress.yaml
```

#### 1.3 策略自动化应用
- [ ] 在 `backend/internal/k8s/` 中创建 `network_policy.go`
- [ ] 实现 `NetworkPolicyManager` 结构体
- [ ] 在服务启动时自动应用基础策略
- [ ] 实现按需应用互联网访问策略

**Go 代码框架：**
```go
// backend/internal/k8s/network_policy.go
type NetworkPolicyManager struct {
    clientset *kubernetes.Clientset
}

func (m *NetworkPolicyManager) EnsureDefaultPolicies(ctx context.Context) error {
    // 应用 base/ 目录下的所有策略
    policies := []string{
        "default-deny-all",
        "allow-dns",
        "deny-k8s-api",
        "allow-gateway-ingress",
    }
    // ...
}

func (m *NetworkPolicyManager) ApplyInternetAccess(ctx context.Context, sandboxID string) error {
    // 为沙箱 Pod 添加 internet-access=true label
    // Cilium 会自动应用 allow-internet-egress 策略
}

func (m *NetworkPolicyManager) RemoveInternetAccess(ctx context.Context, sandboxID string) error {
    // 移除 label，策略自动失效
}
```

#### 1.4 策略持久化验证
- [ ] 验证策略存储在 etcd 中
- [ ] 测试 K3s 重启后策略自动恢复
- [ ] 验证 Cilium Pod 重启后策略仍然有效

**验证命令：**
```bash
# 检查策略存在
kubectl get networkpolicy -n liteboxd

# 重启 K3s 节点

# 等待 Cilium 恢复
kubectl wait --for=condition=ready pod -l k8s-app=cilium -n kube-system

# 验证策略仍然存在
kubectl get networkpolicy -n liteboxd
```

### 验收标准
- [ ] 远程集群 Cilium 状态正常
- [ ] 新创建的 Pod 默认无法访问外部网络
- [ ] Pod 可以进行 DNS 查询
- [ ] Pod 无法访问 K8s API Server
- [ ] K3s 重启后策略自动恢复

---

## Phase 2: 数据模型与 API 扩展

### 目标
扩展数据模型，支持网络配置和访问令牌。

### 任务清单

#### 2.1 数据模型扩展
- [ ] 扩展 `pkg/model/template.go` 中的 `TemplateSpec`
- [ ] 扩展 `pkg/model/sandbox.go` 中的 `Sandbox`
- [ ] 扩展 `pkg/model/sandbox.go` 中的 `CreateSandboxRequest`

**新增类型定义：**
```go
// pkg/model/network.go
type NetworkSpec struct {
    AllowInternetAccess bool     `json:"allowInternetAccess"`
    AllowedDomains      []string `json:"allowedDomains,omitempty"`
}
```

#### 2.2 数据库 Schema 更新
- [ ] 更新 SQLite schema（如需持久化网络配置）
- [ ] 添加迁移脚本

#### 2.3 K8s Client 扩展
- [ ] 在 `internal/k8s/client.go` 中添加令牌生成逻辑
- [ ] 扩展 `CreatePodOptions` 支持 `NetworkSpec`
- [ ] 添加 Pod Label 用于标识互联网访问权限

**实现细节：**
```go
// internal/k8s/client.go
const (
    AnnotationAccessToken = "liteboxd.io/access-token"
    LabelInternetAccess   = "liteboxd.io/internet-access"
)

type CreatePodOptions struct {
    // ... 现有字段
    Network          *NetworkSpec
}

func (c *Client) CreatePod(ctx context.Context, opts CreatePodOptions) (*corev1.Pod, error) {
    // 生成访问令牌
    accessToken := generateAccessToken()

    // 添加到 Annotation
    annotations[AnnotationAccessToken] = accessToken

    // 添加互联网访问 Label
    if opts.Network != nil && opts.Network.AllowInternetAccess {
        labels[LabelInternetAccess] = "true"
    }

    // ...
}
```

#### 2.4 Handler 扩展
- [ ] 扩展 `internal/handler/sandbox.go` 创建接口
- [ ] 返回 `AccessToken` 和 `AccessURL` 字段

#### 2.5 Service 扩展
- [ ] 扩展 `internal/service/sandbox.go` 支持网络配置
- [ ] 处理 `NetworkOverride`

### 验收标准
- [ ] 创建沙箱时返回访问令牌
- [ ] 令牌存储在 Pod Annotation 中
- [ ] API 响应包含 `accessToken` 和 `accessUrl`
- [ ] 单元测试覆盖新增功能

---

## Phase 3: 网关服务实现

### 目标
实现网关服务，处理外部流量转发和令牌认证。

### 任务清单

#### 3.1 网关服务框架
- [ ] 创建 `cmd/gateway/main.go`
- [ ] 创建 `internal/gateway/` 目录
- [ ] 设计中间件链

**目录结构：**
```
backend/
├── cmd/
│   └── gateway/
│       └── main.go
├── internal/
│   └── gateway/
│       ├── handler.go
│       ├── middleware.go
│       ├── proxy.go
│       └── config.go
```

#### 3.2 认证中间件
- [ ] 实现 Token 验证逻辑
- [ ] 从 Pod Annotation 获取存储的令牌
- [ ] 返回适当的错误状态码

**实现框架：**
```go
// internal/gateway/middleware.go
func AuthMiddleware(k8sClient *k8s.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        sandboxID := c.Param("sandbox")
        token := c.GetHeader("X-Access-Token")

        if token == "" {
            c.AbortWithStatusJSON(401, gin.H{"error": "missing access token"})
            return
        }

        // 验证令牌
        valid, err := verifyToken(k8sClient, sandboxID, token)
        if !valid || err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "invalid access token"})
            return
        }

        c.Next()
    }
}
```

#### 3.3 代理转发
- [ ] 实现 HTTP 请求转发
- [ ] 支持 WebSocket 透传
- [ ] 处理超时和连接错误

**实现框架：**
```go
// internal/gateway/proxy.go
type ProxyHandler struct {
    k8sClient *k8s.Client
}

func (p *ProxyHandler) ProxyRequest(c *gin.Context) {
    sandboxID := c.Param("sandbox")
    port := c.Param("port")

    // 获取 Pod IP
    podIP, err := p.k8sClient.GetPodIP(c, sandboxID)
    if err != nil {
        c.AbortWithStatusJSON(404, gin.H{"error": "sandbox not found"})
        return
    }

    // 构建目标 URL
    targetURL := fmt.Sprintf("http://%s:%s", podIP, port)

    // 转发请求
    // ...
}
```

#### 3.4 WebSocket 支持
- [ ] 检测 WebSocket Upgrade 请求
- [ ] 建立客户端 WebSocket 连接
- [ ] 建立上游 WebSocket 连接（直连 Pod IP）
- [ ] 建立上游 WebSocket 连接（K8s API Server Proxy）
- [ ] 双向转发与关闭回收
- [ ] 子协议与必要 Header 透传
- [ ] 连接超时与保活策略

#### 3.5 K8s Service 与 Ingress
- [ ] 创建网关服务的 K8s Service
- [ ] 配置 Ingress 规则（如使用 Traefik/Nginx Ingress）

**K8s 资源：**
```yaml
# deploy/gateway-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: liteboxd-gateway
  namespace: liteboxd
spec:
  selector:
    app: liteboxd-gateway
  ports:
  - port: 8080
    targetPort: 8080
```

#### 3.6 配置管理
- [ ] 网关域名配置
- [ ] 请求超时配置
- [ ] 日志级别配置

### 验收标准
- [ ] 网关服务成功启动
- [ ] 带有效令牌的请求成功转发到沙箱
- [ ] 无效令牌返回 401
- [ ] WebSocket 连接正常工作
- [ ] 沙箱不存在返回 404

---

## Phase 4: 集成测试与文档

### 目标
完善测试覆盖，编写使用文档。

### 任务清单

#### 4.1 集成测试
- [ ] 端到端测试：创建沙箱 → 获取令牌 → 访问服务
- [ ] 网络隔离测试：验证默认拒绝出网
- [ ] 令牌认证测试：验证各种认证场景
- [ ] WebSocket 测试

#### 4.2 性能测试
- [ ] 网关转发延迟测试
- [ ] 并发连接测试
- [ ] 资源占用测试

#### 4.3 文档编写
- [ ] 用户指南：如何使用网络访问功能
- [ ] API 文档更新
- [ ] 运维文档：CNI 部署与配置

#### 4.4 示例代码
- [ ] Go SDK 示例
- [ ] cURL 示例

### 验收标准
- [ ] 所有测试通过
- [ ] 文档完整清晰
- [ ] 示例代码可运行

---

## Phase 5+: 可选增强功能

### 5.1 域名白名单
- [ ] 设计白名单匹配算法
- [ ] 实现基于白名单的 HTTP 代理
- [ ] 支持通配符域名

### 5.2 TCP 端口访问
- [ ] 评估技术方案（SNI Proxy / SOCKS）
- [ ] 实现 TCP 网关
- [ ] 安全风险评估

### 5.3 高级功能
- [ ] 速率限制
- [ ] 访问审计日志
- [ ] 实时流量监控

---

## 实施顺序建议

### 第一步：Phase 1 + Phase 2（并行）
- 准备远程 K3s + Cilium 集群
- 实现 Network Policy 管理器
- 扩展数据模型

### 第二步：Phase 3
- 实现网关服务
- 集成认证和代理

### 第三步：Phase 4
- 完善测试和文档
- 验证策略持久化

### 后续：Phase 5+
- 根据需求实现增强功能

---

## 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Cilium 安装失败 | 高 | 添加重试逻辑，提供手动安装回退方案 |
| Network Policy 语法错误 | 中 | 使用 kubectl dry-run 验证 |
| 策略重启后丢失 | 中 | 已缓解：Cilium 策略存储在 etcd，自动持久化 |
| 网关性能瓶颈 | 中 | 添加水平扩展支持 |
| 令牌泄露风险 | 高 | 文档强调安全实践，考虑短期令牌 |

---

## 依赖清单

### 外部依赖
- **Cilium v1.18.6** - CNI 和网络策略执行引擎
- **Cilium CLI** - 用于安装和管理
- Traefik/Nginx Ingress Controller (可选)

### 内部依赖
- 现有 k8s Client
- 现有 Template Service
- 现有 Sandbox Service

### 远程集群依赖
```
K3s + Cilium (远程)
  │
  └──► backend / gateway 连接 KUBECONFIG
```
