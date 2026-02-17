# Fluent Bit -> CLS 部署清单

本目录提供 `liteboxd-system` 的日志采集清单：

- 采集 `liteboxd-api`、`liteboxd-gateway` 容器 stdout
- 通过 `fluent-bit-go-cls` 插件投递到腾讯云 CLS

## 前置准备

1. 应用日志已启用 JSON stdout（`deploy/system/configmap.yaml`）：
   - `LOG_FORMAT=json`
   - `LOG_OUTPUT=stdout`
2. 已构建并推送包含 `/fluent-bit/plugins/fluent-bit-go.so` 的 Fluent Bit 自定义镜像
3. 已准备 CLS 的 `SecretId/SecretKey`、`TopicID`、`CLSEndPoint`

## 部署步骤

### 1) 修改清单占位符

- `daemonset.yaml` 里的镜像：
  - `REPLACE_WITH_FLUENT_BIT_CLS_IMAGE`
- `configmap.yaml` 里的 CLS 参数：
  - `CLS_ENDPOINT`
  - `CLS_TOPIC_ID`

### 2) 创建凭据 Secret

推荐命令行创建：

```bash
kubectl -n liteboxd-system create secret generic cls-credentials \
  --from-literal=secretId='<你的SecretId>' \
  --from-literal=secretKey='<你的SecretKey>'
```

或者复制 `secret.example.yaml` 改值后手动 apply（不要把真实凭据提交到仓库）。

### 3) 应用清单

```bash
kubectl apply -k deploy/observability/fluent-bit-cls/
```

### 4) 验证

```bash
kubectl -n liteboxd-system get ds fluent-bit-cls
kubectl -n liteboxd-system get pods -l app=fluent-bit-cls
kubectl -n liteboxd-system logs -l app=fluent-bit-cls --tail=200
```

## 注意事项

- 生产环境建议使用 Secret 管理系统（External Secrets/Vault）下发 `cls-credentials`
- 若集群运行时日志格式非 CRI，请调整 `parsers.conf` 与 `[INPUT] Parser`
- 若你还需要采集其他命名空间或应用，可调整 `Path` 与 `grep` 过滤规则
