# LiteBoxd 部署文件

本目录包含 LiteBoxd 的部署配置文件。

## 文件说明

| 文件/目录 | 描述 |
|----------|------|
| `system/` | 控制面部署（`liteboxd-system`），包含 api/gateway、PVC、RBAC |
| `sandbox/` | 沙箱面部署（`liteboxd-sandbox`），包含 NetworkPolicy 与跨命名空间 RBAC |
| `rolling-upgrade.md` | 控制面滚动升级操作指南（升级、验证、回滚） |
| `gateway.yaml` | 旧版单文件部署（已不推荐） |
| `network-policies/` | 旧版网络策略目录（已不推荐） |

## 远程 K3s 集群准备

在独立机器上部署 K3s 和 Cilium，不要在 Docker 容器内安装 K3s。

参考文档：
https://docs.cilium.io/en/stable/installation/k3s/

### 安装 K3s（Server）

```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC='--flannel-backend=none --disable-network-policy' sh -
```

### 安装 K3s Agent（可选）

```bash
curl -sfL https://get.k3s.io | K3S_URL='https://${MASTER_IP}:6443' K3S_TOKEN=${NODE_TOKEN} sh -
```

### 配置 KUBECONFIG

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
```

### 安装 Cilium

```bash
CILIUM_CLI_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/cilium-cli/main/stable.txt)
CLI_ARCH=amd64
if [ "$(uname -m)" = "aarch64" ]; then CLI_ARCH=arm64; fi
curl -L --fail --remote-name-all https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-${CLI_ARCH}.tar.gz{,.sha256sum}
sha256sum --check cilium-linux-${CLI_ARCH}.tar.gz.sha256sum
sudo tar xzvfC cilium-linux-${CLI_ARCH}.tar.gz /usr/local/bin
rm cilium-linux-${CLI_ARCH}.tar.gz{,.sha256sum}
```

```bash
cilium install --version 1.18.6 --set=ipam.operator.clusterPoolIPv4PodCIDRList="10.42.0.0/16"
```

### 验证集群

```bash
cilium status --wait
cilium connectivity test
```

## 连接远程集群

将远程机器的 kubeconfig 拷贝到本机后，设置环境变量：

```bash
export KUBECONFIG=~/.kube/config
```

## Phase1 部署（推荐）

### 1. 准备 K8s 集群

确保集群支持 Network Policy（推荐使用 Cilium）。

### 1.1 构建并推送镜像（示例）

**方式一：使用一键构建脚本（推荐）**

在项目根目录执行，指定镜像仓库和标签：

```bash
# 仅构建
REGISTRY=<your-registry> TAG=phase1 ./deploy/build.sh

# 构建并推送
REGISTRY=<your-registry> TAG=phase1 PUSH=true ./deploy/build.sh
```

`REGISTRY` 必填；`TAG` 默认 `latest`；`PUSH=true` 时会在构建后执行 `docker push`。

**方式二：手动构建**

```bash
# API 镜像
docker build -f backend/Dockerfile -t <your-registry>/liteboxd-server:phase1 ./backend
docker push <your-registry>/liteboxd-server:phase1

# Gateway 镜像
docker build -f backend/Dockerfile.gateway -t <your-registry>/liteboxd-gateway:phase1 ./backend
docker push <your-registry>/liteboxd-gateway:phase1
```

推荐使用脚本注入镜像，不改仓库 YAML：

```bash
REGISTRY=<your-registry> TAG=phase1 \
bash deploy/scripts/deploy-k8s.sh
```

脚本会按固定镜像名自动拼接：

- `${REGISTRY}/liteboxd-server:${TAG}`
- `${REGISTRY}/liteboxd-gateway:${TAG}`

并在临时 kustomize overlay 里覆盖后执行 `kubectl apply`，仓库内 `deploy/system/*.yaml` 不会被改动。

### 2. 部署控制面

```bash
kubectl apply -k deploy/system/
```

### 3. 部署沙箱面（RBAC + 网络策略）

```bash
kubectl apply -k deploy/sandbox/
```

### 4. 验证部署

```bash
# 检查控制面服务
kubectl get pods -n liteboxd-system

# 检查沙箱命名空间策略
kubectl get networkpolicy -n liteboxd-sandbox

# 检查跨命名空间 RBAC 绑定
kubectl get rolebinding -n liteboxd-sandbox
```

## 从集群外访问

当前 `deploy/system` 中 `liteboxd-api` 与 `liteboxd-gateway` 都是 `ClusterIP`，默认仅集群内可达。  
可选两种方式：

### 方式 A：Ingress（推荐）

`deploy/system/ingress.yaml` 已提供示例路由（k3s 默认 Traefik）：

- `/api/v1/sandbox/*` -> `liteboxd-gateway:8081`
- `/api/v1/*` -> `liteboxd-api:8080`

默认 host 为 `liteboxd.local`，请按你的环境改成真实域名。

如果要让创建沙箱后返回的 `accessUrl` 可被外部访问，请把 `deploy/system/configmap.yaml` 里的 `GATEWAY_URL` 改成外部地址，例如：

```yaml
GATEWAY_URL: "https://liteboxd.example.com"
```

### 方式 B：一键端口转发（开发调试）

```bash
make port-forward-k8s
```

或直接执行脚本：

```bash
bash deploy/scripts/port-forward-k8s.sh
```

默认会转发：

- `http://127.0.0.1:8080` -> `liteboxd-api:8080`
- `http://127.0.0.1:8081` -> `liteboxd-gateway:8081`

## 环境变量

| 变量 | 默认值 | 描述 |
|------|--------|------|
| `KUBECONFIG` | 空 | 本地调试时可设置；Pod 内默认使用 in-cluster 配置 |
| `CONTROL_NAMESPACE` | liteboxd-system | 控制面命名空间 |
| `SANDBOX_NAMESPACE` | liteboxd-sandbox | 沙箱命名空间 |
| `PORT` | 8080 | API 服务端口 |
| `PORT` (gateway) | 8081 | 网关服务端口 |
| `SHUTDOWN_TIMEOUT` | 120s（部署清单中） | 优雅停机与会话排空超时时间 |
| `GATEWAY_URL` | http://liteboxd-gateway.liteboxd-system.svc.cluster.local:8081 | API 返回给客户端的网关访问地址 |
| `DATA_DIR` | ./data | 数据目录 |

## Phase2 热升级行为

- `livenessProbe` 使用 `/health`（进程活性）
- `readinessProbe` 使用 `/readyz`（draining 时返回 503），探测周期为 2 秒，失败阈值为 1
- 收到 `SIGTERM` 后：
  1. 先进入 draining，停止接收新请求/会话
  2. 触发 readiness 失败，让 Pod 从 Service endpoints 摘除
  3. 等待已有 WebSocket 会话排空，直到 `SHUTDOWN_TIMEOUT`
- 部署里配置了 `preStop sleep 5`，为 endpoint 摘除传播预留缓冲
- Ingress 已启用 Traefik retry middleware（`attempts: 2`，`initialInterval: 100ms`），降低滚动升级期间短暂 503 的用户感知

## 故障排查

### Cilium 安装失败

```bash
cilium status
```

### 网络策略未生效

```bash
# 检查 Cilium 状态
cilium status

# 查看策略列表
kubectl get networkpolicy -n liteboxd-sandbox -o yaml

# 检查 Pod 标签
kubectl get pods -n liteboxd-sandbox --show-labels
```

### 网关无法访问沙箱

```bash
# 检查网络策略
kubectl get networkpolicy -n liteboxd-sandbox

# 检查网关 Pod 状态
kubectl get pods -n liteboxd-system -l app=liteboxd-gateway

# 测试沙箱网络连通性
kubectl exec -it <pod-name> -n liteboxd-sandbox -- sh
```
