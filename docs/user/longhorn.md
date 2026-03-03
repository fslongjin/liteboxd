# Longhorn 部署指南（集成 liteboxd-installer）

本文说明如何通过 `liteboxd-installer` 自动安装/升级 Longhorn。

## 1. 配置

在 installer 配置文件中加入（或修改）：

```yaml
storage:
  longhorn:
    enabled: true
    namespace: "longhorn-system"
    releaseName: "longhorn"
    chartRepoURL: "https://charts.longhorn.io"
    chartVersion: ""
    defaultReplicaCount: 1
    setDefaultStorageClass: true
    helmInstallScriptURL: "https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3"
```

字段说明：

- `enabled`：是否启用 Longhorn 安装流程
- `chartVersion`：留空表示跟随仓库最新 chart；建议生产环境固定版本
- `defaultReplicaCount`：单机建议 `1`，多节点按可用性需求调整
- `setDefaultStorageClass`：是否将 Longhorn 设为默认 StorageClass
- `helmInstallScriptURL`：master 上缺少 Helm 时使用该脚本安装 Helm

## 2. 执行安装

全量安装（含 LiteBoxd）：

```bash
./bin/liteboxd-installer -f /path/to/install.yaml apply
```

仅准备集群（不部署 LiteBoxd）：

```bash
./bin/liteboxd-installer -f /path/to/cluster-only.yaml apply --cluster-only
```

## 3. 行为说明

- installer 会先在 `master + agents` 节点安装 Longhorn 依赖（`open-iscsi`）
- 然后通过 Helm 执行 `upgrade --install`
- 配置未变化时会跳过 Longhorn 升级步骤（幂等）

## 4. 验证

```bash
kubectl get sc
kubectl -n longhorn-system get pods
```

预期：

- 存在 `longhorn` StorageClass
- `longhorn-system` 下组件状态正常

## 5. 示例文件

- 全量示例：`tools/liteboxd-installer/examples/install.example.yaml`
- 集群模式示例：`tools/liteboxd-installer/examples/cluster-only.example.yaml`
