# liteboxd-installer

`liteboxd-installer` 是 LiteBoxd 一键部署工具（MVP）。

## 功能

- 基于 YAML 配置一键安装主节点 K3s
- 按配置并发加入 Agent 节点
- 固定安装/升级 Cilium
- 可选安装/升级 Longhorn（含 open-iscsi 依赖检查与安装）
- 支持 K3s 国内镜像安装模式（`INSTALL_K3S_MIRROR=cn`）
- 支持下发每台机器的 `/etc/rancher/k3s/registries.yaml`，配置变化自动重启 `k3s/k3s-agent`
- 使用 `liteboxd.configDir` 的 kustomize patch 目录机制部署 LiteBoxd
- 支持镜像自定义
- 支持增量扩容（更新 `cluster.agents` 后重新 `apply`）
- 支持显式删节点（`node remove`）

## 构建

在仓库根目录：

```bash
make build-installer
```

二进制输出：`bin/liteboxd-installer`

## 用法

```bash
# 全量部署/幂等重入
./bin/liteboxd-installer -f tools/liteboxd-installer/examples/install.example.yaml apply

# 仅管理集群（不部署 liteboxd；可包含 Longhorn）
./bin/liteboxd-installer -f tools/liteboxd-installer/examples/cluster-only.example.yaml apply --cluster-only

# 输出详细执行日志到文件（推荐排障时开启）
./bin/liteboxd-installer -f tools/liteboxd-installer/examples/install.example.yaml --log-file /tmp/liteboxd-installer.log apply

# 从中断状态重试
./bin/liteboxd-installer -f tools/liteboxd-installer/examples/install.example.yaml resume

# 显式删除节点
./bin/liteboxd-installer -f tools/liteboxd-installer/examples/install.example.yaml node remove --hosts 10.10.10.13
```

## 注意事项

- 首版仅支持 SSH 密码认证
- 不支持整集群 `destroy`
- `network.cni` 必须为 `cilium`
- `storage.longhorn.enabled=true` 时，installer 会在 master+agent 上安装 `open-iscsi` 依赖，并在集群安装/升级 Longhorn
- `storage.longhorn.defaultReplicaCount=1` 适合单机/单副本场景，多节点生产环境建议按可用性要求调整
- `storage.longhorn.setDefaultStorageClass=true` 会将 Longhorn 设为默认 StorageClass
- `storage.longhorn.helmInstallScriptURL` 默认使用 Helm 官方安装脚本；若网络受限可改为你自己的镜像地址
- `cluster.master.host` / `cluster.agents[].host` 用于 SSH 管理地址
- `cluster.master.nodeIP` / `cluster.agents[].nodeIP` 用于集群内节点地址（join / kubeconfig / cilium）
- `liteboxd.ingressHost` 可配置 Ingress 域名（默认 `liteboxd.local`）
- `liteboxd.gatewayURL` 可配置返回给业务侧的访问基地址（默认回退到集群内 `liteboxd-gateway` Service 地址）
- `liteboxd.metadataRetentionDays` 可配置沙箱元数据保留天数（默认 7，对应环境变量 `SANDBOX_METADATA_RETENTION_DAYS`）
- `liteboxd.security.sandboxTokenEncryptionKey` / `liteboxd.security.sandboxTokenEncryptionKeyID` 可在 installer 配置中覆盖默认的 token 加密参数（建议用环境变量注入）
- 为安全起见，建议使用环境变量注入密码（`${MASTER_PASS}`）
- `--cluster-only` 模式下可只维护 K3s/Cilium/Longhorn/节点，不执行 LiteBoxd 部署步骤
- `--cluster-only` 模式可不提供 `liteboxd.configDir` 和 `liteboxd.images`
- `cluster.k3sInstall.mirror=cn` 时，`network.cilium.version` 必须 `<=1.18.6`
- 排障时建议加 `--log-file <path>`，会记录每条远端命令及 stdout/stderr（敏感 token 会脱敏）
