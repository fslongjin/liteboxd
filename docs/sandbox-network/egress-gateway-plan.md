# LiteBoxd 沙箱公网统一出口方案（Cilium Egress Gateway + ZeroTier 隧道）

## 1. 背景与目标

当前沙箱运行在 K8s 集群内服务器上。目标：

1. 沙箱容器访问公网时，统一经指定 Egress Gateway 节点，通过 ZeroTier 隧道到达云服务器，再由云服务器出公网。
2. 保持现有沙箱创建/访问流程不变（API/Gateway 入站链路不改）。
3. 从网络层强制路径，不依赖应用层代理变量。
4. 公网出口 IP 固定为云服务器的公网 IP，便于审计和白名单。

目标链路：

```
Sandbox Pod -> Cilium Egress Gateway 节点 -> ZeroTier 隧道 -> 云服务器出口 -> Internet
```

## 2. 当前现状（基于仓库实现）

1. 已有默认拒绝、DNS 放通、网关入站放通等基线策略。
2. `allow-internet-egress` 当前是允许沙箱直接出公网（80/443）。
3. 沙箱是否可公网访问由 `liteboxd.io/internet-access=true` 标签控制。
4. 该标签来自模板 `allowInternetAccess`，无需改调用方 API。

结论：当前是"可出公网"，还不是"必须经指定路径出公网"。

## 3. 方案架构

### 3.1 整体架构图

```
┌───────────────────────────────────────────────────────────────────┐
│                         K8s 集群                                    │
│                                                                    │
│  ┌──────────┐    Cilium Egress    ┌────────────────────────────┐  │
│  │ Sandbox  │ ──── Gateway ─────> │  Egress Gateway 节点        │  │
│  │   Pod    │    (SNAT to ZT IP)  │                            │  │
│  └──────────┘                     │  ┌──────────────────────┐  │  │
│                                   │  │ 策略路由              │  │  │
│                                   │  │ src=ZT_IP -> table   │  │  │
│                                   │  │ 100 -> via 云服务器   │  │  │
│                                   │  └──────┬───────────────┘  │  │
│                                   └─────────┼──────────────────┘  │
│                                             │                      │
└─────────────────────────────────────────────┼──────────────────────┘
                                              │
                                    ZeroTier 隧道 (UDP 9993)
                                              │
┌─────────────────────────────────────────────┼──────────────────────┐
│                       云服务器（出口节点）      │                      │
│                                             ▼                      │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ ZeroTier 接口接收 -> ip_forward -> MASQUERADE NAT          │   │
│  │ src: ZT_IP -> 云服务器公网 IP                                │   │
│  └────────────────────────────┬───────────────────────────────┘   │
│                               │                                    │
│                    公网出口 (eth0 / 云服务器公网 IP)                  │
└───────────────────────────────┼────────────────────────────────────┘
                                │
                                ▼
                            Internet
```

### 3.2 核心原理

1. **Cilium Egress Gateway**：匹配 `app=liteboxd` 且 `liteboxd.io/internet-access=true` 的 Pod，
   将公网目标流量 SNAT 到 egress 节点的 **ZeroTier IP**（`egressIP`）。
2. **源地址策略路由**：egress 节点上，`ip rule` 按源地址（ZeroTier IP）将流量导入专用路由表，
   默认路由指向云服务器的 ZeroTier IP，经 ZeroTier 隧道送出。
3. **云服务器 NAT 转发**：云服务器开启 `ip_forward`，对来自 ZeroTier 子网的流量做 MASQUERADE，
   以云服务器公网 IP 出站。
4. **回程流量**：Internet 响应回到云服务器公网 IP -> conntrack 反向 NAT 回 ZeroTier 隧道 ->
   egress 节点收到 -> Cilium conntrack 反向 SNAT 回原始 Pod。

### 3.3 为什么选择 ZeroTier

| 考量 | ZeroTier | WireGuard | IPsec |
|------|----------|-----------|-------|
| 配置复杂度 | 低（加入网络即可） | 中（需手动配 peer） | 高 |
| NAT 穿透 | 内置 | 需额外配置 | 需额外配置 |
| 多节点扩展 | 简单（加入同一网络） | 需逐对配置 | 需逐对配置 |
| 管理界面 | ZeroTier Central Web UI | 无 | 无 |
| 性能 | 好（P2P 直连时） | 略优 | 好 |

ZeroTier 适合本场景：配置简单、支持 NAT 穿透、管理方便。

## 4. 落地步骤

### 4.1 阶段 A：能力检查

1. 检查 Cilium 版本与组件健康。
2. 检查 Egress Gateway CRD 是否存在。

```bash
cilium status --wait
kubectl get crd | grep -i ciliumegressgateway
kubectl api-resources | grep -i egress
```

若无 CRD，先按 `deploy/README.md` 升级启用：

```bash
cilium upgrade --version 1.18.6 \
  --reuse-values \
  --set egressGateway.enabled=true \
  --set bpf.masquerade=true \
  --set kubeProxyReplacement=true
```

### 4.2 阶段 B：创建 ZeroTier 网络

1. 注册 [ZeroTier Central](https://my.zerotier.com/) 账号。
2. 创建一个新网络，记下 **Network ID**（16 位十六进制字符串，例如 `a1b2c3d4e5f67890`）。
3. 在网络设置中：
   - **Access Control** 设为 `Private`（需要手动授权节点）。
   - **IPv4 Auto-Assign** 选择一个子网，例如 `10.147.17.0/24`。
   - 或使用自定义子网（例如 `172.29.0.0/24`），根据你的网络规划选择。
4. 记下分配的子网 CIDR，后续配置会用到。

> **子网建议**：建议使用与 K8s Pod CIDR（`10.42.0.0/16`）和 Service CIDR 不冲突的网段。
> ZeroTier 默认的 `10.147.17.0/24` 一般不会冲突，可直接使用。

### 4.3 阶段 C：配置云服务器（出口节点）

云服务器是最终的公网出口。需要安装 ZeroTier、开启转发、配置 NAT。

#### C.1 安装 ZeroTier

```bash
# 安装 ZeroTier（官方一键脚本）
curl -s https://install.zerotier.com | sudo bash

# 加入你创建的网络
sudo zerotier-cli join <zt-network-id>

# 查看状态（首次加入会显示 REQUESTING_CONFIGURATION）
sudo zerotier-cli listnetworks
```

然后去 ZeroTier Central 网页上授权这台机器（勾选 `Auth` 复选框）。

授权后再次检查：

```bash
sudo zerotier-cli listnetworks
# 应显示 OK 状态，以及分配到的 ZeroTier IP（例如 10.147.17.2）
```

记下这台云服务器的 **ZeroTier IP**，后续称为 `<cloud-zt-ip>`（例如 `10.147.17.2`）。

#### C.2 开启 IP 转发

```bash
# 临时生效
sudo sysctl -w net.ipv4.ip_forward=1

# 持久化
echo 'net.ipv4.ip_forward = 1' | sudo tee /etc/sysctl.d/99-ip-forward.conf
sudo sysctl -p /etc/sysctl.d/99-ip-forward.conf
```

验证：

```bash
cat /proc/sys/net/ipv4/ip_forward
# 输出应为 1
```

#### C.3 配置 iptables NAT 规则

对来自 ZeroTier 子网的流量做 MASQUERADE（源地址替换为云服务器公网 IP）：

```bash
# 设置环境变量（根据你的实际环境修改）
export ZT_SUBNET="10.147.17.0/24"    # ZeroTier 子网
export WAN_IFACE="eth0"              # 公网出口网卡

# 查看云服务器的公网出口网卡名称（通常是 eth0、ens3、ens5 等）
ip route show default
# 示例输出: default via 10.0.0.1 dev eth0 proto dhcp src 203.0.113.50 metric 100
# 这里的出口网卡是 eth0，对应 WAN_IFACE="eth0"

# 添加 MASQUERADE 规则
sudo iptables -t nat -A POSTROUTING -s $ZT_SUBNET -o $WAN_IFACE -j MASQUERADE

# 允许转发来自 ZeroTier 的流量
sudo iptables -A FORWARD -i zt+ -o $WAN_IFACE -j ACCEPT
sudo iptables -A FORWARD -i $WAN_IFACE -o zt+ -m state --state RELATED,ESTABLISHED -j ACCEPT
```

> **说明**：
> - `ZT_SUBNET`：ZeroTier 网络分配的子网，在 ZeroTier Central 中查看。
> - `WAN_IFACE`：云服务器的公网出口网卡，用 `ip route show default` 确认。
> - `zt+` 匹配所有 ZeroTier 接口（接口名以 `zt` 开头）。
> - MASQUERADE 会自动使用出口网卡的 IP 作为源地址。
> - `RELATED,ESTABLISHED` 规则允许回程流量通过。

#### C.4 持久化 iptables 规则

**方式一：使用 iptables-persistent（Debian/Ubuntu）**

```bash
sudo apt-get install -y iptables-persistent
# 安装时会提示保存当前规则，选择 Yes

# 后续修改规则后手动保存
sudo netfilter-persistent save
```

**方式二：使用 systemd 服务**

创建一个环境变量文件，集中管理配置：

```bash
sudo tee /etc/default/zt-nat << 'EOF'
# ZeroTier NAT 转发配置
ZT_SUBNET=10.147.17.0/24
WAN_IFACE=eth0
EOF
```

创建 systemd 服务：

```bash
sudo tee /etc/systemd/system/zt-nat.service << 'EOF'
[Unit]
Description=ZeroTier NAT forwarding rules
After=zerotier-one.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
EnvironmentFile=/etc/default/zt-nat

# 开启转发
ExecStart=/sbin/sysctl -w net.ipv4.ip_forward=1

# NAT 规则
ExecStart=/sbin/iptables -t nat -A POSTROUTING -s ${ZT_SUBNET} -o ${WAN_IFACE} -j MASQUERADE
ExecStart=/sbin/iptables -A FORWARD -i zt+ -o ${WAN_IFACE} -j ACCEPT
ExecStart=/sbin/iptables -A FORWARD -i ${WAN_IFACE} -o zt+ -m state --state RELATED,ESTABLISHED -j ACCEPT

# 停止时清理
ExecStop=/sbin/iptables -t nat -D POSTROUTING -s ${ZT_SUBNET} -o ${WAN_IFACE} -j MASQUERADE
ExecStop=/sbin/iptables -D FORWARD -i zt+ -o ${WAN_IFACE} -j ACCEPT
ExecStop=/sbin/iptables -D FORWARD -i ${WAN_IFACE} -o zt+ -m state --state RELATED,ESTABLISHED -j ACCEPT

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable zt-nat.service
sudo systemctl start zt-nat.service
```

#### C.5 云服务器防火墙/安全组

确保云服务器的安全组或防火墙：

1. **入站**：允许 UDP 9993（ZeroTier 控制面）。如果有其他节点需要直连，放行即可；
   如果都经 ZeroTier 根服务器中继，UDP 9993 出站即可。
2. **出站**：允许 TCP 80/443（HTTP/HTTPS，沙箱要访问的目标端口）。
   如需其他端口，按需放行。

#### C.6 验证云服务器配置

```bash
# 检查 ZeroTier 状态
sudo zerotier-cli listnetworks

# 检查 ip_forward
cat /proc/sys/net/ipv4/ip_forward

# 检查 iptables 规则
sudo iptables -t nat -L POSTROUTING -n -v
sudo iptables -L FORWARD -n -v

# 检查 ZeroTier 接口
ip addr show | grep zt
```

### 4.4 阶段 D：配置 K8s Egress Gateway 节点

#### D.1 安装 ZeroTier 并加入网络

在选定的 egress gateway 节点上：

```bash
# 安装 ZeroTier
curl -s https://install.zerotier.com | sudo bash

# 加入同一个 ZeroTier 网络
sudo zerotier-cli join <zt-network-id>
```

去 ZeroTier Central 授权该节点。授权后：

```bash
sudo zerotier-cli listnetworks
# 记下该节点的 ZeroTier IP，例如 10.147.17.1
```

记下这台 egress 节点的 **ZeroTier IP**，后续称为 `<egress-zt-ip>`（例如 `10.147.17.1`）。

#### D.2 验证 ZeroTier 隧道连通性

```bash
# 从 egress 节点 ping 云服务器的 ZeroTier IP
ping -c 3 <cloud-zt-ip>

# 从云服务器 ping egress 节点的 ZeroTier IP
ping -c 3 <egress-zt-ip>
```

两端都应能 ping 通。

#### D.3 配置策略路由

核心思路：Cilium 会将沙箱流量 SNAT 为 `<egress-zt-ip>` 作为源地址。
我们用 `ip rule` 按源地址匹配，将这些流量导入专用路由表。
该路由表用 `throw` 路由排除私网段（回落到主路由表正常处理），只有公网目标才走 ZeroTier 隧道。

```bash
# 设置环境变量（根据实际环境修改）
export ZT_IFACE="ztxxxxxxxx"         # ZeroTier 接口名（ip link show | grep zt）
export CLOUD_ZT_IP="10.147.17.2"     # 云服务器 ZeroTier IP
export EGRESS_ZT_IP="10.147.17.1"    # 本节点 ZeroTier IP

# 1. 添加自定义路由表（如果尚未添加）
grep -q 'zerotier-egress' /etc/iproute2/rt_tables || \
  echo '100 zerotier-egress' | sudo tee -a /etc/iproute2/rt_tables

# 2. 填充路由表
#    throw = 本表不处理该目的地，回落到主路由表（与 Cilium excludedCIDRs 对应）
sudo ip route add throw 10.0.0.0/8       table zerotier-egress
sudo ip route add throw 172.16.0.0/12    table zerotier-egress
sudo ip route add throw 192.168.0.0/16   table zerotier-egress
sudo ip route add throw 127.0.0.0/8      table zerotier-egress
sudo ip route add throw 169.254.0.0/16   table zerotier-egress

#    只有公网目标走 ZeroTier 隧道到云服务器
sudo ip route add default via $CLOUD_ZT_IP dev $ZT_IFACE table zerotier-egress

# 3. 添加策略路由规则
sudo ip rule add from $EGRESS_ZT_IP table zerotier-egress priority 100
```

> **原理**：
> - Cilium SNAT 后，数据包 src=`<egress-zt-ip>`，dst=公网 IP。
> - `ip rule from <egress-zt-ip>` 匹配，查 `zerotier-egress` 表。
> - dst 在私网段（10/8、172.16/12、192.168/16 等）→ 命中 `throw` → **回落到主路由表**，走本地网络，不受影响。
> - dst 是公网 IP → 命中 `default` → 经 ZeroTier 隧道发往云服务器。
> - 节点自身流量（src=物理 IP）不匹配 `ip rule`，完全不受影响。

验证路由配置：

```bash
# 查看策略路由规则
ip rule show
# 应看到: 100: from <egress-zt-ip> lookup zerotier-egress

# 查看路由表
ip route show table zerotier-egress
# 应看到:
#   throw 10.0.0.0/8
#   throw 172.16.0.0/12
#   throw 192.168.0.0/16
#   throw 127.0.0.0/8
#   throw 169.254.0.0/16
#   default via <cloud-zt-ip> dev ztXXXXXXXX

# 验证：私网目标应走主路由表
ip route get 192.168.199.1 from $EGRESS_ZT_IP
# 应走物理网卡，不走 zt 接口

# 验证：公网目标应走 zerotier-egress 表
ip route get 8.8.8.8 from $EGRESS_ZT_IP
# 应显示 via <cloud-zt-ip> dev ztXXXX table zerotier-egress
```

#### D.4 持久化策略路由

创建环境变量文件：

```bash
sudo tee /etc/default/zt-egress-route << 'EOF'
# ZeroTier egress 路由配置
CLOUD_ZT_IP=10.147.17.2
EGRESS_ZT_IP=10.147.17.1
EOF
```

创建 systemd 服务，确保重启后自动恢复路由规则：

```bash
sudo tee /etc/systemd/system/zt-egress-route.service << 'EOF'
[Unit]
Description=ZeroTier egress policy routing for LiteBoxd
After=zerotier-one.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
EnvironmentFile=/etc/default/zt-egress-route

# 添加路由表条目（幂等：如果已存在则跳过）
ExecStartPre=/bin/bash -c 'grep -q "zerotier-egress" /etc/iproute2/rt_tables || echo "100 zerotier-egress" >> /etc/iproute2/rt_tables'

# 等待 ZeroTier 接口就绪（最多等 30 秒）
ExecStartPre=/bin/bash -c 'for i in $(seq 1 30); do ip link show | grep -q "zt" && break; sleep 1; done'

# 填充路由表：throw 私网段（回落主路由表），default 走 ZeroTier 隧道
ExecStart=/bin/bash -c '\
  ZT_IFACE=$(ip link show | grep -o "zt[a-z0-9]*" | head -1) && \
  ip route replace throw 10.0.0.0/8       table zerotier-egress && \
  ip route replace throw 172.16.0.0/12    table zerotier-egress && \
  ip route replace throw 192.168.0.0/16   table zerotier-egress && \
  ip route replace throw 127.0.0.0/8      table zerotier-egress && \
  ip route replace throw 169.254.0.0/16   table zerotier-egress && \
  ip route replace default via ${CLOUD_ZT_IP} dev $ZT_IFACE table zerotier-egress && \
  ip rule del from ${EGRESS_ZT_IP} table zerotier-egress 2>/dev/null; \
  ip rule add from ${EGRESS_ZT_IP} table zerotier-egress priority 100'

# 清理
ExecStop=/bin/bash -c '\
  ip rule del from ${EGRESS_ZT_IP} table zerotier-egress 2>/dev/null; \
  ip route flush table zerotier-egress 2>/dev/null; \
  true'

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable zt-egress-route.service
sudo systemctl start zt-egress-route.service
```

#### D.5 给 egress 节点打标签

每个 egress 节点都打上通用标签，方便运维查询和管理：

```bash
kubectl label node <node-name> liteboxd.io/role=egress --overwrite
```

### 4.5 阶段 E：应用 Cilium Egress Gateway 策略

编辑 `deploy/sandbox/egress-gateway/cilium-egress-gateway-policy.yaml`。

策略使用 `egressGateways`（列表形式），每个 egress 节点一个条目，各自配置自己的 ZeroTier IP。
Cilium 根据 Pod 的 CiliumEndpoint UID 将其分配到某一个 gateway（Pod 生命周期内稳定不变）。

```yaml
apiVersion: cilium.io/v2
kind: CiliumEgressGatewayPolicy
metadata:
  name: liteboxd-sandbox-egress-via-node
spec:
  selectors:
    - podSelector:
        matchLabels:
          app: liteboxd
          liteboxd.io/internet-access: "true"
  destinationCIDRs:
    - 0.0.0.0/0
  excludedCIDRs:
    - 10.0.0.0/8
    - 172.16.0.0/12
    - 192.168.0.0/16
    - 127.0.0.0/8
    - 169.254.0.0/16
  egressGateways:
    # 每个 egress 节点一个条目，用 hostname 唯一选择节点
    - nodeSelector:
        matchLabels:
          kubernetes.io/hostname: <egress-node-1-hostname>
      egressIP: <egress-node-1-zt-ip>   # 例如 10.147.17.1
    # 扩容时新增条目即可
    # - nodeSelector:
    #     matchLabels:
    #       kubernetes.io/hostname: <egress-node-2-hostname>
    #   egressIP: <egress-node-2-zt-ip>   # 例如 10.147.17.3
```

> **关键**：
> - 每个节点的 `egressIP` 必须是该节点的 ZeroTier IP，用于触发策略路由走 ZeroTier 隧道。
> - 用 `kubernetes.io/hostname` 精确选中单个节点（因为每个节点的 ZeroTier IP 不同，不能共用 nodeSelector）。
> - 扩容只需：新节点装 ZeroTier + 配策略路由 + 在此列表追加条目。
> - Cilium 按 CiliumEndpoint UID 分配 Pod 到 gateway，某个 gateway 下线时会自动重新分配。

应用策略：

```bash
kubectl apply -k deploy/sandbox/egress-gateway/
```

### 4.6 阶段 F：与当前 deploy 结构集成

部署文件位于 `deploy/sandbox/egress-gateway/`：

1. `cilium-egress-gateway-policy.yaml` — Cilium 策略
2. `kustomization.yaml` — Kustomize 配置
3. `README.md` — 部署说明

部署命令：

```bash
kubectl apply -k deploy/sandbox/egress-gateway/
```

## 5. 数据包流转详解

### 5.1 出站流程

```
1. Sandbox Pod (10.42.x.x) 发送请求到 1.2.3.4:443
2. Cilium eBPF 匹配 Egress Gateway 策略
   → SNAT: src 10.42.x.x -> <egress-zt-ip> (例如 10.147.17.1)
   → 将数据包送到 egress 节点的网络栈
3. egress 节点内核路由决策:
   → ip rule: from 10.147.17.1 lookup zerotier-egress
   → dst 1.2.3.4 不匹配任何 throw 路由（私网段）
   → 命中 default via 10.147.17.2 dev ztXXXX
   → 数据包经 ZeroTier 隧道发送到云服务器
4. 云服务器收到: src=10.147.17.1 dst=1.2.3.4
   → MASQUERADE: src 10.147.17.1 -> <cloud-public-ip>
   → 从 eth0 发出到 Internet
```

> 如果 dst 是私网地址（例如 192.168.199.x），则命中 `throw 192.168.0.0/16`，
> 回落到主路由表，走物理网卡，不经 ZeroTier 隧道。

### 5.2 回程流程

```
1. Internet 响应: src=1.2.3.4 dst=<cloud-public-ip>
   → 到达云服务器 eth0
2. 云服务器 conntrack 反向 NAT:
   → dst <cloud-public-ip> -> 10.147.17.1
   → 经 ZeroTier 隧道发回 egress 节点
3. egress 节点收到: src=1.2.3.4 dst=10.147.17.1
   → Cilium conntrack 反向 SNAT:
   → dst 10.147.17.1 -> 10.42.x.x (原始 Pod IP)
   → 数据包回到 Sandbox Pod
```

## 6. 验收标准

### 6.1 功能验收

1. `allowInternetAccess=false` 的沙箱不能访问公网。
2. `allowInternetAccess=true` 的沙箱可访问公网。
3. 沙箱执行 `curl -s ifconfig.me` 返回 **云服务器的公网 IP**（而非集群节点 IP）。

### 6.2 链路验收

1. 在 egress 节点上抓包（ZeroTier 接口），确认沙箱公网流量经过 ZeroTier 隧道。

```bash
# 在 egress 节点上
sudo tcpdump -i zt+ -n host <cloud-zt-ip> -c 20
```

2. 在云服务器上抓包，确认流量到达并被 NAT。

```bash
# 在云服务器上
sudo tcpdump -i zt+ -n src <egress-zt-ip> -c 20
```

### 6.3 安全验收

1. 只有被策略选中的沙箱 Pod 走 egress gateway -> ZeroTier -> 云服务器。
2. 其他 Pod 的公网流量不受影响。
3. 节点自身的管理流量（SSH、K8s API 等）不走 ZeroTier 隧道。

### 6.4 稳定性验收

1. 并发场景下链路稳定，无明显丢包和重传异常。
2. ZeroTier 断连后沙箱公网访问中断（fail-closed），不应回落为直出。
3. ZeroTier 恢复后链路自动恢复。
4. 策略回滚后可恢复到当前直接出公网模式。

## 7. 风险与规避

| 风险 | 影响 | 规避措施 |
|------|------|----------|
| ZeroTier 隧道中断 | 沙箱无法访问公网 | 监控 ZeroTier 连接状态，配置告警 |
| 云服务器宕机 | 沙箱无法访问公网 | 准备备用云服务器，配置健康检查和自动切换 |
| 单 egress 节点成为单点 | 该节点故障影响所有沙箱 | 多 egress 节点 + ZeroTier 多路径 |
| 带宽瓶颈 | ZeroTier 隧道或云服务器带宽成为瓶颈 | 监控带宽，按需升级云服务器 |
| ZeroTier 延迟增加 | 沙箱访问公网延迟上升 | 选择离集群近的云服务器区域 |
| 路由规则丢失（重启） | 流量不走 ZeroTier | 使用 systemd 服务持久化路由规则 |

## 8. 故障排查

### 8.1 沙箱无法访问公网

```bash
# 1. 检查 Cilium egress gateway 策略状态
kubectl get ciliumegressgatewaypolicy -o yaml

# 2. 检查 egress 节点上的策略路由
ip rule show
ip route show table zerotier-egress

# 3. 检查 ZeroTier 连接状态
sudo zerotier-cli listnetworks    # 两端都执行
sudo zerotier-cli listpeers       # 查看对端状态

# 4. 测试 ZeroTier 隧道连通性
ping -c 3 <cloud-zt-ip>           # 从 egress 节点
ping -c 3 <egress-zt-ip>          # 从云服务器

# 5. 检查云服务器 NAT 规则
sudo iptables -t nat -L POSTROUTING -n -v
cat /proc/sys/net/ipv4/ip_forward
```

### 8.2 出口 IP 不是云服务器 IP

```bash
# 检查 Cilium SNAT 是否生效（在 egress 节点上）
sudo tcpdump -i zt+ -n -c 10

# 检查策略路由是否命中
ip -s rule show

# 确认 egressIP 配置正确（应为 egress 节点的 ZeroTier IP）
kubectl get ciliumegressgatewaypolicy -o yaml | grep egressIP
```

### 8.3 ZeroTier 连接问题

```bash
# 检查 ZeroTier 服务状态
sudo systemctl status zerotier-one

# 查看 ZeroTier 日志
sudo journalctl -u zerotier-one -f

# 检查对端连接方式（DIRECT 或 RELAY）
sudo zerotier-cli listpeers
# DIRECT 表示直连（延迟低），RELAY 表示经中继（延迟高）
# 如果持续 RELAY，检查双方 UDP 9993 是否放行
```

## 9. 注意事项

1. **单节点集群**：如果 K8s 只有一个节点，该节点同时是 egress 节点，
   Cilium SNAT + 策略路由仍然生效，流量仍会走 ZeroTier 隧道。
2. **ZeroTier 子网选择**：选择的 ZeroTier 子网不应与 K8s Pod CIDR（`10.42.0.0/16`）
   和 Service CIDR 冲突。默认的 `10.147.17.0/24` 通常是安全的。
3. **excludedCIDRs 中已包含私网段**：Cilium 策略的 `excludedCIDRs` 排除了 `10.0.0.0/8` 等私网段，
   这意味着沙箱 Pod 直接访问 ZeroTier IP 的流量不会被 SNAT。
   这是正确的行为——只有公网目标流量才走 egress gateway。
4. **ZeroTier 控制面流量**：ZeroTier 自身的 UDP 9993 控制面流量使用节点物理 IP，
   不会被策略路由影响。
5. **回滚**：删除 Cilium 策略和策略路由即可恢复原始直出模式：
   ```bash
   kubectl delete -k deploy/sandbox/egress-gateway/
   sudo ip rule del from <egress-zt-ip> table zerotier-egress
   sudo ip route flush table zerotier-egress
   ```

## 10. 建议实施顺序

1. 先配置云服务器（安装 ZeroTier、开启转发、配置 NAT）。
2. 在 egress 节点安装 ZeroTier 并验证隧道连通性。
3. 在 egress 节点配置策略路由。
4. 在测试命名空间创建一个 `allowInternetAccess=true` 的沙箱灰度验证。
5. 验证 `curl ifconfig.me` 返回云服务器公网 IP。
6. 验证故障场景（ZeroTier 断连、云服务器不可用）后再全量推广。

## 附录 A：配置速查表

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `<zt-network-id>` | ZeroTier 网络 ID | `a1b2c3d4e5f67890` |
| `<egress-zt-ip>` | egress 节点的 ZeroTier IP | `10.147.17.1` |
| `<cloud-zt-ip>` | 云服务器的 ZeroTier IP | `10.147.17.2` |
| `<cloud-public-ip>` | 云服务器的公网 IP | `203.0.113.50` |
| `<zt-interface>` | ZeroTier 网络接口名 | `ztXXXXXXXX` |
| `<node-name>` | K8s egress 节点名 | `k3s-worker-1` |
| ZeroTier 子网 | ZeroTier 分配的子网 | `10.147.17.0/24` |

## 附录 B：完整配置检查清单

### 云服务器

- [ ] ZeroTier 已安装并加入网络
- [ ] ZeroTier Central 已授权该节点
- [ ] `ip_forward=1` 已持久化
- [ ] iptables MASQUERADE 规则已添加
- [ ] iptables FORWARD 规则已添加
- [ ] iptables 规则已持久化（iptables-persistent 或 systemd）
- [ ] 安全组放行 UDP 9993 入站
- [ ] 安全组放行 TCP 80/443 出站

### Egress 节点

- [ ] ZeroTier 已安装并加入网络
- [ ] ZeroTier Central 已授权该节点
- [ ] 能 ping 通云服务器的 ZeroTier IP
- [ ] 路由表 `zerotier-egress` 已创建
- [ ] 默认路由已添加到 `zerotier-egress` 表
- [ ] `ip rule from <egress-zt-ip>` 已配置
- [ ] 策略路由已通过 systemd 持久化
- [ ] 节点已打标签 `liteboxd.io/role=egress`

### Cilium 策略

- [ ] Egress Gateway CRD 可用
- [ ] `egressIP` 设置为 egress 节点的 ZeroTier IP
- [ ] 策略已 apply 且状态正常
- [ ] 沙箱 `curl ifconfig.me` 返回云服务器公网 IP
