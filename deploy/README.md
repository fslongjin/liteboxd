# LiteBoxd 部署文件

本目录包含 LiteBoxd 的部署配置文件。

## 文件说明

| 文件/目录 | 描述 |
|----------|------|
| `gateway.yaml` | 网关服务的 K8s 部署文件 |
| `network-policies/` | 网络策略定义 |

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

## 生产部署

### 1. 准备 K8s 集群

确保集群支持 Network Policy（推荐使用 Cilium）。

### 2. 部署后端服务

```bash
kubectl apply -f deploy/gateway.yaml
```

### 3. 部署网络策略

```bash
kubectl apply -k deploy/network-policies/base/
```

### 4. 验证部署

```bash
# 检查网关服务
kubectl get pods -n liteboxd -l app=liteboxd-gateway

# 检查网络策略
kubectl get networkpolicy -n liteboxd
```

## 环境变量

| 变量 | 默认值 | 描述 |
|------|--------|------|
| `KUBECONFIG` | ~/.kube/config | Kubeconfig 文件路径 |
| `PORT` | 8080 | API 服务端口 |
| `GATEWAY_PORT` | 8081 | 网关服务端口 |
| `GATEWAY_URL` | http://localhost:8080 | 网关外部访问 URL |
| `DATA_DIR` | ./data | 数据目录 |

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
kubectl get networkpolicy -n liteboxd -o yaml

# 检查 Pod 标签
kubectl get pods -n liteboxd --show-labels
```

### 网关无法访问沙箱

```bash
# 检查网络策略
kubectl get networkpolicy -n liteboxd

# 检查网关 Pod 状态
kubectl get pods -n liteboxd -l app=liteboxd-gateway

# 测试沙箱网络连通性
kubectl exec -it <pod-name> -n liteboxd -- sh
```
