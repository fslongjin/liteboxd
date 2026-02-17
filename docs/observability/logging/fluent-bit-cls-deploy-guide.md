# liteboxd 日志采集部署指南（Fluent Bit -> 腾讯云 CLS）

本文给出一套可直接落地的日志采集方案：在 K8s 中部署 Fluent Bit（DaemonSet）采集 `liteboxd-api`、`liteboxd-gateway` 的容器标准输出日志，并转发到腾讯云 CLS。

## 1. 前置条件

- 已完成 `liteboxd` 的基础部署
- `liteboxd-api`、`liteboxd-gateway` 日志为 JSON（当前代码默认支持）
- 已在腾讯云创建 CLS 日志集和 Topic
- 拥有 CLS 写入权限的 `SecretId` / `SecretKey`

## 2. 推荐：直接使用仓库内清单部署

仓库已提供可直接应用的 Kustomize 清单：

- `deploy/observability/fluent-bit-cls/`

使用步骤：

1. 先按本文后续步骤构建并推送带 CLS 插件的 Fluent Bit 镜像
2. 修改 `deploy/observability/fluent-bit-cls/daemonset.yaml` 中镜像占位符
3. 修改 `deploy/observability/fluent-bit-cls/configmap.yaml` 中 `CLS_ENDPOINT` / `CLS_TOPIC_ID`
4. 创建凭据 Secret：

```bash
kubectl -n liteboxd-system create secret generic cls-credentials \
  --from-literal=secretId='<你的SecretId>' \
  --from-literal=secretKey='<你的SecretKey>'
```

5. 部署：

```bash
kubectl apply -k deploy/observability/fluent-bit-cls/
```

## 3. liteboxd 侧日志配置确认

确保 `deploy/system/configmap.yaml` 中至少是以下值（生产建议）：

- `LOG_LEVEL=info`
- `LOG_FORMAT=json`
- `LOG_OUTPUT=stdout`

然后重新部署系统组件：

```bash
kubectl apply -k deploy/system
kubectl -n liteboxd-system rollout status deploy/liteboxd-api
kubectl -n liteboxd-system rollout status deploy/liteboxd-gateway
```

## 4. 创建 CLS 凭据 Secret

> 下面示例将日志采集器部署在 `liteboxd-system` 命名空间。

```bash
kubectl -n liteboxd-system create secret generic cls-credentials \
  --from-literal=secretId='<你的SecretId>' \
  --from-literal=secretKey='<你的SecretKey>'
```

## 5. 构建带 CLS 插件的 Fluent Bit 镜像

腾讯 CLS 常见做法是使用 `fluent-bit-go-cls` 输出插件。该插件默认不在社区版 Fluent Bit 镜像内，需要自定义镜像。

在任意构建目录创建 `Dockerfile`（示例）：

```dockerfile
FROM golang:1.22 AS builder
WORKDIR /build
RUN git clone https://github.com/TencentCloud/fluent-bit-go-cls.git
WORKDIR /build/fluent-bit-go-cls
RUN go build -buildmode=c-shared -o /build/fluent-bit-go.so .

FROM cr.fluentbit.io/fluent/fluent-bit:3.2
COPY --from=builder /build/fluent-bit-go.so /fluent-bit/plugins/fluent-bit-go.so
```

构建并推送（示例）：

```bash
docker build -t <你的镜像仓库>/fluent-bit-cls:latest .
docker push <你的镜像仓库>/fluent-bit-cls:latest
```

## 6. 使用 Helm 安装 Fluent Bit

添加 Helm 仓库：

```bash
helm repo add fluent https://fluent.github.io/helm-charts
helm repo update
```

在本地创建一个 `values-fluent-bit-cls.yaml`（建议放在仓库外或私有配置库）：

```yaml
kind: DaemonSet

image:
  repository: <你的镜像仓库>/fluent-bit-cls
  tag: latest
  pullPolicy: IfNotPresent

serviceAccount:
  create: true
  name: fluent-bit

rbac:
  create: true

env:
  - name: CLS_SECRET_ID
    valueFrom:
      secretKeyRef:
        name: cls-credentials
        key: secretId
  - name: CLS_SECRET_KEY
    valueFrom:
      secretKeyRef:
        name: cls-credentials
        key: secretKey

config:
  service: |
    [SERVICE]
        Flush         1
        Daemon        Off
        Log_Level     info
        Parsers_File  parsers.conf
        Plugins_File  plugins.conf
        HTTP_Server   On
        HTTP_Listen   0.0.0.0
        HTTP_Port     2020

  customParsers: |
    [PLUGINS]
        Path /fluent-bit/plugins/fluent-bit-go.so

  inputs: |
    [INPUT]
        Name              tail
        Tag               kube.*
        Path              /var/log/containers/*.log
        Parser            cri
        DB                /var/log/flb_kube.db
        Mem_Buf_Limit     10MB
        Skip_Long_Lines   On
        Refresh_Interval  5

  filters: |
    [FILTER]
        Name                kubernetes
        Match               kube.*
        Kube_Tag_Prefix     kube.var.log.containers.
        Merge_Log           On
        Keep_Log            Off
        K8S-Logging.Parser  On
        K8S-Logging.Exclude Off

    [FILTER]
        Name    grep
        Match   kube.*
        Regex   $kubernetes['namespace_name'] ^liteboxd-system$

    [FILTER]
        Name    grep
        Match   kube.*
        Regex   $kubernetes['labels']['app'] ^liteboxd-(api|gateway)$

  outputs: |
    [OUTPUT]
        Name                fluent-bit-go-cls
        Match               kube.*
        TopicID             <你的TopicID>
        CLSEndPoint         <你的CLSEndPoint>
        AccessKeyID         ${CLS_SECRET_ID}
        AccessKeySecret     ${CLS_SECRET_KEY}
```

安装或升级：

```bash
helm upgrade --install fluent-bit fluent/fluent-bit \
  -n liteboxd-system \
  --create-namespace \
  -f values-fluent-bit-cls.yaml
```

## 7. 验证采集链路

1) 验证 DaemonSet 就绪：

```bash
kubectl -n liteboxd-system get pods -l app.kubernetes.io/name=fluent-bit
```

2) 看采集器日志是否有鉴权/投递错误：

```bash
kubectl -n liteboxd-system logs -l app.kubernetes.io/name=fluent-bit --tail=200
```

3) 在 CLS Topic 中按以下字段检索：

- `service=liteboxd-server` 或 `service=liteboxd-gateway`
- `status>=400`
- 指定 `request_id`

## 8. 常见问题

### Q1: CLS 收不到日志

- 检查 `SecretId/SecretKey` 是否正确
- 检查 `region/logset/topic` 是否与腾讯云控制台一致
- 检查 grep 过滤条件是否把日志过滤掉了
- 检查 `LOG_FORMAT` 是否为 `json`

### Q2: 日志字段不完整

- 确认应用已升级到结构化日志版本
- 确认 Fluent Bit 开启了 `Merge_Log On`（将容器日志中的 JSON 合并为字段）

### Q3: Helm 部署成功，但 Fluent Bit 报找不到 CLS 插件

- 确认镜像内存在 `/fluent-bit/plugins/fluent-bit-go.so`
- 确认 `Plugins_File plugins.conf` 已配置
- 确认 `[PLUGINS]` 里 `Path` 与镜像路径一致

### Q4: 希望接入其他日志服务

保持应用侧不变（继续 JSON 到 stdout），仅替换 Fluent Bit `outputs` 配置即可（如 ES/Loki/S3 等）。

## 9. 本地开发建议

本地 `make run-backend` / `make run-gateway` 场景通常不需要部署采集器：

- 用 `LOG_OUTPUT=stdout,file` 保留控制台 + 本地滚动文件
- 用 `tail -f ./logs/liteboxd.log` 做本地排查

这样可保持和生产字段一致，同时避免本地引入额外运维组件。
