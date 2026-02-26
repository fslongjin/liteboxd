# Fluent Bit集成OpenObserve指南

本指南介绍如何修改现有的 Fluent Bit ConfigMap，将特定命名空间（例如 `liteboxd-system`）的日志转发到 OpenObserve，而不影响现有的日志收集配置。

## 前提条件

- Kubernetes 集群中已安装 Fluent Bit
- 已部署 OpenObserve 并获取了连接信息（Host, Port, User, Password）

## 操作步骤

### 1. 导出当前配置

首先，将集群中现有的 Fluent Bit ConfigMap 导出到本地文件：

```bash
kubectl -n fluent-bit get configmap fluent-bit -o yaml > fluent-bit.yaml
```

### 2. 编辑配置

使用文本编辑器打开 `fluent-bit.yaml`，找到 `data` -> `fluent-bit.conf` 部分。

在 `[OUTPUT]` 区域（通常在文件末尾），**追加**以下配置片段。

> **注意**：请保留文件中原有的 `[INPUT]`, `[FILTER]` 和其他 `[OUTPUT]` 配置，只在最后添加以下内容。

```conf
    # 新增 OpenObserve 输出
    [OUTPUT]
        Name                http
        # 使用 Match_Regex 精确匹配特定命名空间 (这里是 liteboxd-system)
        # Kubernetes 日志 Tag 格式通常为: kube.var.log.containers.<pod>_<namespace>_<container>-<id>.log
        # 下面的正则匹配包含 _liteboxd-system_ 的 Tag
        Match_Regex         kube\..*_liteboxd-system_.*
        
        # OpenObserve 连接配置
        # URI 格式: /api/<organization>/<stream>/_json
        # 请替换 <organization> (默认是 default) 和 <stream> (例如 dev-lj-liteboxd-system)
        URI                 /api/<organization>/<stream>/_json
        
        # OpenObserve 服务地址 (不带 http/https)
        Host                <OPENOBSERVE_HOST>
        Port                <OPENOBSERVE_PORT>
        
        # 如果是 HTTPS 请设为 On，HTTP 请设为 Off
        tls                 Off
        
        # 数据格式配置
        Format              json
        Json_date_key       _timestamp
        Json_date_format    iso8601
        
        # 认证信息
        HTTP_User           <OPENOBSERVE_USER>
        HTTP_Passwd         <OPENOBSERVE_PASS>
        
        # 压缩配置
        compress            gzip
```

### 3. 替换占位符

请务必将上述配置中的以下占位符替换为实际值：

- `<organization>`: 你的 OpenObserve 组织名称，默认为 `default`
- `<stream>`: 你希望写入的流名称，例如 `dev-lj-liteboxd-system`
- `<OPENOBSERVE_HOST>`: OpenObserve 服务器地址（例如 `openobserve.example.com` 或 IP）
- `<OPENOBSERVE_PORT>`: OpenObserve 端口（例如 `5080` 或 `443`）
- `<OPENOBSERVE_USER>`: 用户名（通常是邮箱）
- `<OPENOBSERVE_PASS>`: 密码或 Token

### 4. 应用配置

保存文件后，将更新后的 ConfigMap 应用到集群：

```bash
kubectl apply -f fluent-bit.yaml
```

### 5. 重启 Fluent Bit

为了让配置生效，需要重启 Fluent Bit 的 DaemonSet：

```bash
kubectl -n fluent-bit rollout restart daemonset fluent-bit
```

或者，如果你的环境不支持 `rollout restart`，可以删除 Pods 让其自动重建：

```bash
kubectl -n fluent-bit delete pods -l app.kubernetes.io/name=fluent-bit
```

## 验证

配置生效后，Fluent Bit 会自动将 `liteboxd-system` 命名空间的日志发送到 OpenObserve。你可以登录 OpenObserve 控制台，在对应的 Stream 中查看日志。
