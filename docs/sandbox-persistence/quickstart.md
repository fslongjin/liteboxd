# 持久化沙箱快速部署指南（Longhorn）

本文给你一条最短可执行路径：  
在 K3s 上部署 Longhorn，然后在 LiteBoxd 中创建 `persistent-rootfs` 沙箱。

## 1. 前置条件

1. 你已有可用的 LiteBoxd（建议用 `liteboxd-installer` 部署）。
2. 集群节点满足 Longhorn 前置要求（尤其是 `open-iscsi`）。
3. `kubectl` 可访问该集群。

## 2. 安装 Longhorn

推荐方式：通过 `liteboxd-installer` 配置项自动安装。

在 installer YAML 中启用：

```yaml
storage:
  longhorn:
    enabled: true
    defaultReplicaCount: 1
    setDefaultStorageClass: true
```

然后执行：

```bash
./bin/liteboxd-installer -f /path/to/install.yaml apply
```

如果你需要手动安装，可使用下述命令：

> 单机集群可用；建议默认副本数设为 `1`。

```bash
helm repo add longhorn https://charts.longhorn.io
helm repo update

kubectl create namespace longhorn-system || true

helm upgrade --install longhorn longhorn/longhorn \
  -n longhorn-system \
  --set defaultSettings.defaultReplicaCount=1
```

手动安装后检查状态：

```bash
kubectl -n longhorn-system get pods
kubectl get sc
```

确保有 `longhorn` StorageClass 可用。

## 3. 给 LiteBoxd API 增加 PVC/Deployment 权限

本仓库已更新沙箱 RBAC：  
`tools/liteboxd-installer/internal/installer/assets/deploy/sandbox/rbac-api.yaml`

如果你是用 installer 部署，重新执行一次 `apply` 即可下发最新权限。  
如果你是手动部署，重新 `kubectl apply -k deploy/sandbox`。

## 4. 导入持久化模板（example）

仓库已提供示例模板：

- `templates/code-interpreter-persistent.yml`

导入：

```bash
liteboxd import --file templates/code-interpreter-persistent.yml
```

## 5. 创建持久化沙箱

```bash
liteboxd sandbox create --template code-interpreter-persistent --wait
```

说明：

- 模板里 `ttl: 0`，表示永久运行（不自动过期）。
- 持久卷默认 `1Gi`，StorageClass 为 `longhorn`。

## 6. 验证“重建后数据仍在”

1. 在沙箱写入 rootfs：

```bash
liteboxd sandbox exec <sandbox-id> -- sh -lc "echo hello-persist > /root/persist.txt"
```

2. 删除运行中的 Pod（不删除 sandbox 元数据）：

```bash
kubectl -n liteboxd-sandbox delete pod -l sandbox-id=<sandbox-id>
```

3. 等待新 Pod 就绪后读取：

```bash
liteboxd sandbox exec <sandbox-id> -- cat /root/persist.txt
```

若输出 `hello-persist`，说明重建后持久化生效。

## 7. 常见问题

### Q1：为什么不用 local-path？

`local-path` 对 PVC `size` 通常不提供硬限制保证，无法满足“每沙箱磁盘上限”这个硬需求。  
`persistent-rootfs` 模式默认要求 Longhorn（或同类具备硬配额的 CSI）。

### Q2：单机能用吗？

可以。单机建议 Longhorn 默认副本数 `1`；多节点可按可用性要求提升副本数。

### Q3：如何清理数据？

- 删除 sandbox 时，`reclaimPolicy: Delete` 会删除 PVC。  
- 若改为 `Retain`，删除 sandbox 后 PVC 仍会保留。
