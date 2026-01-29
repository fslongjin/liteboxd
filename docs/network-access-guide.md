# LiteBoxd 网络访问功能

## 概述

LiteBoxd 提供安全的沙箱网络访问功能，包括：
- **默认拒绝出网访问**：沙箱默认无法访问外部网络
- **令牌强制认证**：所有入站访问必须通过令牌认证
- **HTTP 代理**：通过网关服务访问沙箱内部服务
- **可配置的互联网访问**：通过模板配置启用

## 快速开始

### 1. 准备远程 K3s + Cilium

在独立机器上部署 K3s 和 Cilium，不要在 Docker 容器内安装 K3s。
参考文档：
https://docs.cilium.io/en/stable/installation/k3s/

将远程 kubeconfig 拷贝到本机并设置环境变量：

```bash
export KUBECONFIG=~/.kube/config
```

### 2. 启动后端与网关

```bash
make run-backend
make run-gateway
```

### 3. 创建带网络配置的模板

```bash
curl -X POST http://localhost:8080/api/v1/templates \
  -H "Content-Type: application/json" \
  -d '{
    "name": "python-internet",
    "displayName": "Python with Internet Access",
    "description": "Python template with outbound internet access",
    "spec": {
      "image": "python:3.11-slim",
      "resources": {
        "cpu": "500m",
        "memory": "512Mi"
      },
      "ttl": 3600,
      "network": {
        "allowInternetAccess": true
      }
    }
  }'
```

### 4. 创建沙箱

```bash
curl -X POST http://localhost:8080/api/v1/sandboxes \
  -H "Content-Type: application/json" \
  -d '{
    "template": "python-internet"
  }'
```

响应示例：
```json
{
  "id": "abc12345",
  "image": "python:3.11-slim",
  "status": "pending",
  "accessToken": "1a2b3c4d5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t1u2v3w4x5y6z",
  "accessUrl": "http://localhost:8080/api/v1/sandbox/abc12345",
  "createdAt": "2025-01-24T10:00:00Z",
  "expiresAt": "2025-01-24T11:00:00Z"
}
```

### 5. 访问沙箱服务

```bash
curl http://localhost:8081/api/v1/sandbox/abc12345/port/3000/ \
  -H "X-Access-Token: 1a2b3c4d5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t1u2v3w4x5y6z"
```

## API 参考

### 模板网络配置

在模板 spec 中添加 `network` 字段：

```json
{
  "network": {
    "allowInternetAccess": true,
    "allowedDomains": ["api.example.com", "github.com"]
  }
}
```

| 字段 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `allowInternetAccess` | boolean | `false` | 是否允许访问互联网 |
| `allowedDomains` | string[] | - | 域名白名单（未来功能） |

### 沙箱访问令牌

创建沙箱后，响应中包含：
- `accessToken`: 用于访问沙箱的令牌
- `accessUrl`: 网关访问 URL

**重要**：令牌仅在创建时返回，请妥善保管。

### 网关访问 API

**URL 格式**：`/api/v1/sandbox/{sandbox-id}/port/{port}/{path}`

**认证 Header**：`X-Access-Token: {token}`

**支持方法**：GET、POST、PUT、DELETE、PATCH

## 网络策略

### 默认策略（自动应用）

```yaml
# 1. 默认拒绝所有入站和出站流量
default-deny-all

# 2. 允许 DNS 查询
allow-dns

# 3. 拒绝访问 K8s API
deny-k8s-api

# 4. 允许网关入站
allow-gateway-ingress
```

### 互联网访问策略

当模板中 `allowInternetAccess: true` 时，自动应用：
```yaml
allow-internet-egress
```

允许访问：
- `0.0.0.0/0` 排除私有网络段
- TCP 端口 80 (HTTP)
- TCP 端口 443 (HTTPS)

拒绝访问：
- `10.0.0.0/8` (私有网络)
- `172.16.0.0/12` (私有网络)
- `192.168.0.0/16` (私有网络)
- `127.0.0.0/8` (本地回环)
- `169.254.0.0/16` (链路本地)

## 环境变量

| 变量 | 默认值 | 描述 |
|------|--------|------|
| `PORT` | 8080 | API 服务端口 |
| `GATEWAY_PORT` | 8081 | 网关服务端口 |
| `GATEWAY_URL` | - | 网关外部访问 URL |
| `KUBECONFIG` | ~/.kube/config | Kubeconfig 路径 |

## 故障排查

### 网关返回 401
- 检查 `X-Access-Token` Header 是否正确
- 确认令牌与创建沙箱时返回的匹配

### 网关返回 404
- 确认沙箱 ID 正确
- 检查沙箱是否仍在运行（可能已过期）

### 无法访问互联网
- 确认模板中 `allowInternetAccess: true`
- 检查 Cilium 是否正常运行：`kubectl get pods -n kube-system | grep cilium`
- 查看网络策略：`kubectl get networkpolicy -n liteboxd`

## 示例

### 创建 HTTP 服务器沙箱

```bash
# 1. 创建模板
cat > template.json << 'EOF'
{
  "name": "http-server",
  "displayName": "HTTP Server",
  "description": "Simple HTTP server template",
  "spec": {
    "image": "nginx:alpine",
    "resources": {
      "cpu": "200m",
      "memory": "256Mi"
    },
    "ttl": 1800,
    "network": {
      "allowInternetAccess": false
    }
  }
}
EOF

curl -X POST http://localhost:8080/api/v1/templates \
  -H "Content-Type: application/json" \
  -d @template.json

# 2. 创建沙箱
RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/sandboxes \
  -H "Content-Type: application/json" \
  -d '{"template": "http-server"}')

SANDBOX_ID=$(echo $RESPONSE | jq -r '.id')
ACCESS_TOKEN=$(echo $RESPONSE | jq -r '.accessToken')

# 3. 启动一个简单的 HTTP 服务器
curl -X POST http://localhost:8080/api/v1/sandbox/$SANDBOX_ID/exec \
  -H "Content-Type: application/json" \
  -d '{"command": ["sh", "-c", "echo \"Hello from Sandbox\" > /usr/share/nginx/html/index.html && nginx"]}' \
  --max-time 120

# 4. 访问沙箱服务
curl http://localhost:8081/api/v1/sandbox/$SANDBOX_ID/port/80/ \
  -H "X-Access-Token: $ACCESS_TOKEN"
```

### Python HTTP 服务器示例

```bash
# 1. 创建 Python 模板（带互联网访问）
curl -X POST http://localhost:8080/api/v1/templates \
  -H "Content-Type: application/json" \
  -d '{
    "name": "python-web",
    "displayName": "Python Web Server",
    "spec": {
      "image": "python:3.11-slim",
      "resources": {"cpu": "500m", "memory": "512Mi"},
      "ttl": 3600,
      "network": {"allowInternetAccess": true},
      "startupScript": "pip install flask"
    }
  }'

# 2. 创建沙箱并访问
# ... (类似上面的步骤)
```

## 安全注意事项

1. **令牌安全**：访问令牌仅在创建时返回，请妥善保管
2. **默认隔离**：未配置互联网访问的沙箱无法访问外网
3. **基础设施保护**：沙箱无法访问 K8s API Server
4. **令牌认证**：所有网关访问必须通过令牌认证
