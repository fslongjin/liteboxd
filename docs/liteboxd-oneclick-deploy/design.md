# LiteBoxd 一键部署工具设计方案（已确认 v1）

## 1. 目标与范围

### 1.1 目标

基于一个配置文件，在任意可 SSH 访问主节点的机器上执行一次命令，完成：

1. 安装并初始化主节点 K3s。
2. 按配置把子节点自动加入 K3s 集群（支持 IP/用户名/密码）。
3. 按指定的 LiteBoxd 配置目录，把 LiteBoxd 部署到该集群。
4. 支持在配置文件中自定义 LiteBoxd 镜像。

### 1.2 非目标（本期）

1. 不做跨云多集群编排。
2. 不做自动弹性扩缩容（但支持手动增量加节点/删节点）。
3. 不做复杂资产托管（如集中密码管理服务）。

## 2. 方案总览

建议新增一个独立 CLI（示例名：`liteboxd-installer`），核心模块如下：

1. `config-loader`：解析并校验 YAML 配置。
2. `ssh-executor`：通过 SSH 在主/子节点执行命令（首版仅支持密码认证）。
3. `k3s-orchestrator`：负责主节点安装、获取 token、子节点 join。
4. `liteboxd-deployer`：复用仓库 `deploy/system` 和 `deploy/sandbox` 基础清单，叠加镜像覆盖与用户配置目录后部署。
5. `state-recorder`：记录执行状态，支持幂等重试。

执行位置：工具运行在“操作者机器”，真正安装动作在远端节点通过 SSH 执行。

## 3. 配置文件设计

示例：`install.yaml`

```yaml
cluster:
  name: "liteboxd-prod"
  master:
    host: "10.10.10.11"
    port: 22
    user: "root"
    password: "******"
    sudo: false
    k3s:
      version: "v1.30.6+k3s1"
      tlsSAN:
        - "10.10.10.11"
        - "k3s.example.internal"
      installArgs:
        - "--flannel-backend=none"
        - "--disable-network-policy"
  agents:
    - host: "10.10.10.12"
      port: 22
      user: "root"
      password: "******"
      sudo: false
    - host: "10.10.10.13"
      port: 22
      user: "root"
      password: "******"
      sudo: false

network:
  cni: "cilium" # 固定值，首版不支持 none
  cilium:
    version: "1.18.6"
    podCIDR: "10.42.0.0/16"
    kubeProxyReplacement: true
    enableEgressGateway: true

liteboxd:
  namespaceSystem: "liteboxd-system"
  namespaceSandbox: "liteboxd-sandbox"
  configDir: "./deploy-config/prod"
  images:
    api: "registry.example.com/liteboxd/liteboxd-server:v0.2.0"
    gateway: "registry.example.com/liteboxd/liteboxd-gateway:v0.2.0"
    web: "registry.example.com/liteboxd/web:v0.2.0"
  deploySandboxResources: true

runtime:
  parallelism: 5
  sshTimeoutSeconds: 15
  commandTimeoutSeconds: 1200
  removeAbsentAgents: false
  dryRun: false
```

### 3.1 关键字段说明

1. `cluster.master/agents`：覆盖你的主/子节点 IP、用户名、密码诉求。
2. `liteboxd.configDir`：指向 LiteBoxd 部署自定义目录。
3. `liteboxd.images`：可直接覆盖 API/Gateway/Web 镜像。
4. `network.cni`：首版固定为 `cilium`。
5. `runtime.removeAbsentAgents`：是否按配置收敛删除“已不在 agents 列表中的节点”，默认 `false`（安全优先）。

## 4. LiteBoxd 配置目录约定

`liteboxd.configDir` 建议约定如下（避免与代码耦合）：

```text
deploy-config/prod/
  system/
    patches/
      api-env.yaml
      gateway-resources.yaml
  sandbox/
    patches/
      networkpolicy-extra.yaml
  values.env
```

合并策略：

1. 基础清单来自仓库：`deploy/system`、`deploy/sandbox`。
2. 安装器在临时目录生成 kustomize overlay。
3. 先注入 `liteboxd.images` 镜像覆盖，再应用 `configDir` 的 patch。
4. 最终执行 `kubectl apply -k <overlay>`。

这样可保证：

1. 与现有仓库部署资产一致。
2. 每个环境只维护差异配置。
3. 镜像和配置解耦，支持快速发布。

## 5. 端到端执行流程

### Phase 0：预检

1. 校验 YAML 与必填项。
2. 校验主节点 SSH 连通性。
3. 校验每个子节点 SSH 连通性。
4. 校验远端具备必要命令（`curl`、`systemctl`）。

### Phase 1：主节点安装 K3s

1. 若未安装 `k3s`，按配置执行安装脚本。
2. 读取 `/var/lib/rancher/k3s/server/node-token`。
3. 读取并修正 `/etc/rancher/k3s/k3s.yaml` 的 server 地址（从 `127.0.0.1` 替换为主节点可达地址）。

### Phase 2：安装 Cilium（必选）

1. 在主节点安装 `cilium` CLI（若缺失）。
2. 执行 `cilium install` 或 `cilium upgrade --reuse-values`。
3. 等待 `cilium status --wait` 成功。

### Phase 3：子节点加入集群

1. 并发在每个 agent 执行 K3s 安装命令：
   `K3S_URL=https://<master>:6443 K3S_TOKEN=<token> ...`
2. 回主节点执行 `kubectl get nodes` 校验节点 Ready。

### Phase 3.5：节点收敛（增量删节点，可选）

1. 计算“集群当前 agent 节点集合”与“配置文件 agent 节点集合”差异。
2. 默认仅加不减：缺失节点自动加入，多余节点只告警不删除。
3. 仅当 `runtime.removeAbsentAgents=true` 时执行删节点流程。
4. 删节点顺序：
   - `kubectl cordon <node>`
   - `kubectl drain <node> --ignore-daemonsets --delete-emptydir-data`
   - `kubectl delete node <node>`
   - SSH 到目标机器执行 `k3s-agent-uninstall.sh`（若存在）

### Phase 4：部署 LiteBoxd

1. 将仓库 `deploy/` 与用户 `liteboxd.configDir` 打包上传到主节点临时目录。
2. 生成 overlay，注入镜像覆盖与 patch。
3. 执行：
   - `kubectl apply -k <tmp>/system-overlay`
   - `kubectl apply -k <tmp>/sandbox-overlay`（可开关）
4. 执行 rollout 检查（api/gateway/web）。

### Phase 5：收尾与产物

1. 输出部署摘要（节点、镜像、命名空间、服务状态）。
2. 可选导出 kubeconfig 到本地指定路径。
3. 清理远端临时目录。

## 6. 幂等、失败处理与回滚

### 6.1 幂等原则

1. K3s 已安装则跳过安装，仅做健康检查。
2. Agent 已在集群则跳过 join。
3. LiteBoxd 使用 `kubectl apply`，重复执行可收敛。
4. 增量扩容通过更新 `cluster.agents` 后再次 `apply` 即可完成。

### 6.2 失败处理

1. 子节点失败不影响主节点，支持失败节点重试。
2. 部署失败时输出失败资源与事件（`kubectl describe` + `kubectl get events`）。
3. 生成 `state.json`，支持 `resume` 从断点继续。
4. 删节点失败时立即停止后续删除，防止误扩散。

### 6.3 回滚策略

1. LiteBoxd 回滚：保留上一次成功 overlay 快照，失败可 `kubectl apply -k <last-good>`。
2. K3s 层不提供整集群 `destroy`，避免误删生产节点。

## 7. 安全设计

1. 密码字段支持环境变量引用（如 `${MASTER_PASS}`），避免明文落盘。
2. 日志脱敏：密码/token 永不打印。
3. 首版仅密码认证；后续版本再扩展 SSH 私钥认证。
4. 如需 `sudo`，支持独立 `sudoPassword`，默认复用 `password`。

## 8. 工程落地建议

### 8.1 代码位置

建议新增：

```text
tools/liteboxd-installer/
  cmd/
  internal/config
  internal/ssh
  internal/k3s
  internal/deploy
  internal/state
```

### 8.2 CLI 形态

```bash
liteboxd-installer apply -f install.yaml
liteboxd-installer apply -f install.yaml --dry-run
liteboxd-installer resume -f install.yaml
liteboxd-installer node remove -f install.yaml --hosts 10.10.10.13
```

说明：

1. 扩容：修改 `cluster.agents` 后执行 `apply`。
2. 缩容：推荐执行 `node remove` 显式删节点；或在 `removeAbsentAgents=true` 下使用声明式收敛删除。

## 9. MVP 范围（第一阶段）

1. 支持 1 主 + N 子节点（SSH 密码认证）。
2. 支持 K3s 安装 + Agent 自动加入。
3. 固定支持 Cilium 安装。
4. 支持基于 `liteboxd.configDir` + 镜像覆盖部署 LiteBoxd。
5. 支持失败重试与基础状态恢复。
6. 支持便捷增量扩容与安全缩容（显式删节点 + 可选声明式收敛）。

## 10. 已确认决策

1. `liteboxd.configDir` patch 机制：按 kustomize patch 目录约定推进。
2. SSH 认证：首版仅支持密码认证，不支持私钥。
3. 命令范围：首版支持 `apply/resume/node remove`，不提供整集群 `destroy`。
4. CNI：首版固定 Cilium，不支持 `none` 模式。
