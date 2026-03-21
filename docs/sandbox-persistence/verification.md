# 持久化验证手册（LiteBoxd）

本文只回答一个问题：  
如何验证“沙箱 rootfs 持久化”确实生效。

## 1. 先明确验证目标

你要验证的不是“删除 sandbox 后数据是否还在”，而是：

1. Pod/容器重建后，rootfs 写入还在。
2. 删除 sandbox 时，PVC 是否按 `reclaimPolicy` 行为执行（Delete/Retain）。

## 2. 前置检查

```bash
kubectl get sc
kubectl -n longhorn-system get pods
```

建议结果：

- `longhorn` 可用
- `longhorn-system` 核心组件 Running/Ready

## 3. 验证 A：Pod 重建后数据仍在（核心）

1. 创建持久化沙箱（建议 `ttl=0`）：

```bash
liteboxd sandbox create --template <template-name> --ttl 0 --wait
```

2. 向 rootfs 写入测试文件：

```bash
liteboxd sandbox exec <sandbox-id> -- sh -lc "echo hello-persist > /root/persist.txt && sync"
```

3. 先读一次确认写入成功：

```bash
liteboxd sandbox exec <sandbox-id> -- cat /root/persist.txt
```

4. 删除 Pod（注意：不是删除 sandbox）：

```bash
kubectl -n liteboxd-sandbox delete pod -l sandbox-id=<sandbox-id>
kubectl -n liteboxd-sandbox get pod -l sandbox-id=<sandbox-id> -w
```

5. 新 Pod 就绪后再次读取：

```bash
liteboxd sandbox exec <sandbox-id> -- cat /root/persist.txt
```

若仍输出 `hello-persist`，说明持久化有效。

## 4. 验证 B：容量限制生效

在沙箱内持续写入直到超过 PVC 配额：

```bash
liteboxd sandbox exec <sandbox-id> -- sh -lc "dd if=/dev/zero of=/root/fill.bin bs=1M count=102400"
```

预期：

- 超过配额后写入失败（No space left on device）。
- 不影响其他 sandbox 的配额。

## 5. 验证 C：删除 sandbox 时 PVC 回收策略

### 5.1 `reclaimPolicy=Delete`

删除 sandbox 后，PVC 应被删除：

```bash
liteboxd sandbox delete <sandbox-id>
kubectl -n liteboxd-sandbox get pvc | grep sandbox-data-<sandbox-id>
```

预期：查不到该 PVC。

### 5.2 `reclaimPolicy=Retain`

删除 sandbox 后，PVC 仍保留：

```bash
liteboxd sandbox delete <sandbox-id>
kubectl -n liteboxd-sandbox get pvc | grep sandbox-data-<sandbox-id>
```

预期：仍能看到该 PVC。

## 6. 常见误区

1. 误区：删除 sandbox 后还期望“自动恢复同一数据”
- 当前实现是“每 sandbox 一份 PVC”，sandbox 删除后是生命周期结束。
- `Retain` 只是保留 PVC，便于人工审计/恢复，不是自动复用到新 sandbox。

2. 误区：用“重启宿主机”替代“重建 Pod”验证
- 推荐先做 Pod 重建验证，路径更短、定位更直接。

3. 误区：同时存在多个 default StorageClass
- 可能导致未显式指定 `storageClassName` 的 PVC 行为不确定。
- 建议只保留一个 default（通常是 `longhorn`）。

## 7. 卡在 Init 阶段的排查

如果 Pod 长时间停在 `Init:0/1`，优先检查 `rootfs-prepare`：

```bash
kubectl -n liteboxd-sandbox logs <pod-name> -c rootfs-prepare --previous
kubectl -n liteboxd-sandbox logs <pod-name> -c rootfs-prepare
kubectl -n liteboxd-sandbox logs <pod-name> -c rootfs-helper --previous
kubectl -n liteboxd-sandbox logs <pod-name> -c rootfs-helper
kubectl -n liteboxd-sandbox describe pod <pod-name>
```

重点关注：

1. `overlay mount failed`
2. `No space left on device`
3. `pod has unbound immediate PersistentVolumeClaims`
4. `helper failed to mount rootfs`

说明：

- 当前持久化实现是“`rootfs-helper` 进入主容器 mount namespace 后，在 PVC 上组装 overlay upper/work 并回写 ready/unmounted 标记”，不会再把子挂载传播到宿主机 kubelet volume path。  
- PVC 仍然只承载 overlay 可写层（upper/work），不会再整份拷贝 rootfs 到 PVC。  
- 若出现 `No space left on device`，通常是业务写入超过 PVC 配额，而不是初始化拷贝导致。

## 8. 删除卡住与 server 重启恢复验证

### 8.1 删除请求不会被误报为完成

1. 在 Web 控制台删除一个持久化 sandbox。
2. 观察主列表中该 sandbox 消失。
3. 打开 metadata/detail 页面，确认其仍显示：
   - `lifecycle_status=terminating`
   - `deletion.phase` 为非空

### 8.2 模拟 PVC 删除收敛

1. 删除一个 `reclaimPolicy=Delete` 的持久化 sandbox。
2. 执行：

```bash
kubectl -n liteboxd-sandbox get pvc | grep sandbox-data-<sandbox-id>
```

预期：

1. 删除初期可能短暂仍能看到 PVC。
2. 随后 PVC 被 LiteBoxd 后台清理掉。

### 8.3 server 重启后的继续删除

1. 删除一个持久化 sandbox。
2. 在删除尚未完全收敛前重启 `liteboxd-server`。
3. 重启后检查 metadata/detail：
   - `deletion.phase` 继续推进
4. 再检查：

```bash
kubectl -n liteboxd-sandbox get deploy,pod,pvc | grep <sandbox-id>
```

预期：

1. server 重启不会让删除停在中间状态。
2. 后续 Deployment、Pod、PVC 仍会继续收敛直至消失。
