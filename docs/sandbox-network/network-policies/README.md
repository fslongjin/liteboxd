# Network Policy 资源定义

本目录包含 LiteBoxd 沙箱网络隔离所需的 Network Policy 资源。

## 使用方法

```bash
# 应用所有基础策略
kubectl apply -f base/

# 为特定沙箱启用互联网访问
kubectl apply -f internet/allow-internet-egress.yaml
```

## 策略说明

### base/default-deny-all.yaml
默认拒绝所有入站和出站流量，实现"默认拒绝"的安全策略。

### base/allow-dns.yaml
允许沙箱进行 DNS 查询，访问 CoreDNS 服务。

### base/deny-k8s-api.yaml
明确拒绝访问 K8s API Server 和内部服务，防止沙箱攻击基础设施。

### base/allow-gateway-ingress.yaml
允许来自网关服务的入站流量，使网关能够转发请求到沙箱。

### internet/allow-internet-egress.yaml
为标记了 `internet-access: "true"` 的沙箱启用互联网访问。
