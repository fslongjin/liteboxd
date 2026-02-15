# LiteBoxd 滚动升级指南

本文档说明如何在当前架构下执行安全滚动升级。

当前前提：
- 控制面命名空间：`liteboxd-system`
- 沙箱命名空间：`liteboxd-sandbox`
- API 使用 `SQLite + PVC` 过渡方案
- Gateway 多副本，已启用 `draining + readiness + Traefik retry`

## 1. 升级原则

1. 升级控制面（api/gateway）不应删除现有沙箱 Pod。
2. 升级期间允许极短暂重试窗口，不应出现长时间中断。
3. 优先保证“停新收旧”：draining 后不再接收新请求，等待会话排空。

## 2. 升级前检查

```bash
kubectl -n liteboxd-system get pods
kubectl -n liteboxd-system get deploy
kubectl -n liteboxd-system get ingress liteboxd
kubectl -n liteboxd-system get middleware
kubectl -n liteboxd-sandbox get pods
```

确认：
- `liteboxd-api`、`liteboxd-gateway` 均 `Available=True`
- Ingress 与 middleware 存在
- 现有 sandbox 运行正常

## 3. 构建并推送新镜像

```bash
REGISTRY=<your-registry> TAG=<new-tag> PUSH=true ./deploy/build.sh
```

例如：

```bash
REGISTRY=<your-registry> TAG=v20260215-1 PUSH=true ./deploy/build.sh
```

## 4. 执行滚动升级（推荐方式）

```bash
REGISTRY=<your-registry> TAG=<new-tag> make deploy-k8s
```

例如：

```bash
REGISTRY=<your-registry> TAG=v20260215-1 make deploy-k8s
```

该命令会：
- 用临时 kustomize overlay 注入固定镜像名：
  - `${REGISTRY}/liteboxd-server:${TAG}`
  - `${REGISTRY}/liteboxd-gateway:${TAG}`
- `kubectl apply` 控制面和沙箱面资源

## 5. 观察滚动过程

```bash
kubectl -n liteboxd-system rollout status deploy/liteboxd-gateway --timeout=5m
kubectl -n liteboxd-system rollout status deploy/liteboxd-api --timeout=5m
kubectl -n liteboxd-system get pods -w
```

可并行做在线检查：

```bash
curl -s -o /dev/null -w "%{http_code}\n" -H "Host: liteboxd.local" http://<ingress-ip>/api/v1/sandboxes
curl -s -o /dev/null -w "%{http_code}\n" -H "Host: liteboxd.local" http://<ingress-ip>/api/v1/sandbox/test/port/8080/
```

说明：
- 第二条无 token 正常返回 `401`
- 升级窗口内若偶发 `503`，Traefik retry 会自动重试

## 6. 升级后验收

```bash
kubectl -n liteboxd-system get pods
kubectl -n liteboxd-system get deploy
kubectl -n liteboxd-system get endpoints liteboxd-api liteboxd-gateway
kubectl -n liteboxd-sandbox get pods
```

重点确认：
- 新 Pod 全部 Ready
- endpoint 已切换到新 Pod
- 现有 sandbox 未被删除

## 7. 回滚方法

### 方式 A：Kubernetes 原生回滚

```bash
kubectl -n liteboxd-system rollout undo deploy/liteboxd-gateway
kubectl -n liteboxd-system rollout undo deploy/liteboxd-api
kubectl -n liteboxd-system rollout status deploy/liteboxd-gateway --timeout=5m
kubectl -n liteboxd-system rollout status deploy/liteboxd-api --timeout=5m
```

### 方式 B：重新部署旧镜像标签

```bash
REGISTRY=<your-registry> TAG=<old-tag> make deploy-k8s
```

## 8. 当前架构注意事项

1. `liteboxd-api` 当前为 1 副本（SQLite+PVC 过渡方案），无法承诺严格零中断。
2. `liteboxd-gateway` 为多副本并有 retry，通常用户侧影响更小。
3. 若需要更强升级连续性，后续建议：
   - 数据层迁移 Postgres
   - API 多副本 + leader election（后台任务主从）

## 9. 常用排障命令

```bash
kubectl -n liteboxd-system describe deploy liteboxd-api
kubectl -n liteboxd-system describe deploy liteboxd-gateway
kubectl -n liteboxd-system logs deploy/liteboxd-api --tail=200
kubectl -n liteboxd-system logs deploy/liteboxd-gateway --tail=200
kubectl -n kube-system logs deploy/traefik --tail=200
```
