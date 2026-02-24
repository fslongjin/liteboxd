# LiteBoxd 沙箱 Egress Gateway（ZeroTier 隧道出口）

本目录用于把 `allowInternetAccess=true` 的沙箱流量，统一从指定 egress 节点经 ZeroTier 隧道到云服务器出公网：

```
Sandbox Pod -> Cilium Egress Gateway 节点 -> ZeroTier 隧道 -> 云服务器出口 -> Internet
```

完整方案设计见 `docs/sandbox-network/egress-gateway-plan.md`。

## 1. 适用前提

1. 集群已安装 Cilium `1.18.6`（或兼容版本），且已启用 Egress Gateway。
2. 已有一台云服务器作为公网出口节点，配置好 ZeroTier + NAT 转发。
3. K8s egress 节点已安装 ZeroTier 并加入同一网络，策略路由已配置。

```bash
# 检查 Cilium 与 Egress Gateway CRD
cilium status --wait
kubectl get crd | grep -i ciliumegressgateway
kubectl api-resources | grep -i CiliumEgressGatewayPolicy
```

如果查不到相关 CRD，先按 `deploy/README.md` 的升级方式启用：

```bash
cilium upgrade --version 1.18.6 \
  --reuse-values \
  --set egressGateway.enabled=true \
  --set bpf.masquerade=true \
  --set "devices={enp1s0,zt4xyqdgpe}" \ # 这里换成你的本地网卡、zerotier网卡的名称
  --set kubeProxyReplacement=true
```

## 2. 部署前参数准备

### 2.1 云服务器（出口节点）

确保以下配置已完成（详细步骤见 `docs/sandbox-network/egress-gateway-plan.md` 阶段 C）：

- ZeroTier 已安装并加入网络，已授权
- `ip_forward=1` 已开启并持久化
- iptables MASQUERADE + FORWARD 规则已配置并持久化
- 安全组放行 UDP 9993 入站、TCP 80/443 出站

### 2.2 K8s Egress 节点

确保以下配置已完成（详细步骤见 `docs/sandbox-network/egress-gateway-plan.md` 阶段 D）：

- ZeroTier 已安装并加入同一网络，已授权
- 能 ping 通云服务器的 ZeroTier IP
- 策略路由已配置（`ip rule from <egress-zt-ip> table zerotier-egress`）
- 策略路由已通过 systemd 持久化

#### 2.2.1 ZeroTier 多网卡场景：关闭 rp_filter（重要）

在 egress gateway 节点上，如果同时存在物理网卡（如 `enp1s0`）和 ZeroTier 网卡（如 `zt*`），可能出现回包被 Linux 反向路径过滤（`rp_filter`）丢弃，表现为：

- Ingress/Service 间歇性超时或请求悬挂
- Cilium `Cluster health` 出现 `1/2 reachable`
- 跨节点 Pod 连通性不稳定

建议在 **egress gateway 节点** 将相关网卡的 `rp_filter` 关闭：

```bash
# 临时生效（重启后失效）
sudo sysctl -w net.ipv4.conf.all.rp_filter=0
sudo sysctl -w net.ipv4.conf.default.rp_filter=0
sudo sysctl -w net.ipv4.conf.enp1s0.rp_filter=0
sudo sysctl -w net.ipv4.conf.zt4xyqdgpe.rp_filter=0
```

持久化配置（按你的实际网卡名替换）：

```bash
cat <<'EOF' | sudo tee /etc/sysctl.d/99-k8s-multihome.conf
net.ipv4.conf.all.rp_filter=0
net.ipv4.conf.default.rp_filter=0
net.ipv4.conf.enp1s0.rp_filter=0
net.ipv4.conf.zt4xyqdgpe.rp_filter=0
EOF

sudo sysctl --system
```

### 2.3 给出口节点打标签

```bash
kubectl label node <node-name> liteboxd.io/role=egress --overwrite
```

## 3. 修改策略文件

编辑 `deploy/sandbox/egress-gateway/cilium-egress-gateway-policy.yaml`：

1. `spec.egressGateway.egressIP`：设为 egress 节点的 **ZeroTier IP**。
   这是关键——Cilium 将沙箱流量 SNAT 到这个 IP，从而触发策略路由走 ZeroTier 隧道。
2. `spec.egressGateway.nodeSelector.matchLabels`：按你的节点标签调整。
3. 如需收紧公网范围，可调整 `destinationCIDRs` / `excludedCIDRs`。

## 4. 按需部署

仅部署 egress gateway 策略：

```bash
kubectl apply -k deploy/sandbox/egress-gateway/
```

验证资源：

```bash
kubectl get ciliumegressgatewaypolicy
kubectl get ciliumegressgatewaypolicy liteboxd-sandbox-egress-via-node -o yaml
```

## 5. 功能验证

1. 创建模板网络配置为 `allowInternetAccess=true` 的沙箱。
2. 在沙箱内执行：

```bash
curl -s ifconfig.me
```

期望结果：返回 IP 为 **云服务器的公网 IP**（而非集群节点 IP）。

同时检查：

1. `allowInternetAccess=false` 的沙箱仍无法访问公网。
2. DNS 与网关入站能力不受影响。
3. `allowInternetAccess=true` 的沙箱可以访问公网任意端口和协议（TCP/UDP/ICMP 等），不受端口限制。

### 链路验证

在 egress 节点抓包确认流量走 ZeroTier：

```bash
sudo tcpdump -i zt+ -n host <cloud-zt-ip> -c 20
```

在云服务器抓包确认流量到达：

```bash
sudo tcpdump -i zt+ -n src <egress-zt-ip> -c 20
```

## 6. 回滚

移除 egress gateway 策略：

```bash
kubectl delete -k deploy/sandbox/egress-gateway/
```

移除后，`allowInternetAccess=true` 的沙箱会回到当前默认策略（直接出公网）。

如需同时清理策略路由（egress 节点上）：

```bash
sudo ip rule del from <egress-zt-ip> table zerotier-egress
sudo ip route flush table zerotier-egress
```

## 7. 注意事项

1. 该策略是 cluster-scoped，不在 `liteboxd-sandbox` 命名空间内创建对象。
2. 单节点集群下，Cilium SNAT + 策略路由仍然生效，流量仍走 ZeroTier 隧道。
3. `egressIP` 必须是 egress 节点的 ZeroTier IP，不是节点物理 IP。
4. 建议至少准备两个 egress 节点，避免单点出口。
5. ZeroTier 隧道断连时为 fail-closed 行为（沙箱无法出公网），不会回落为直出。
