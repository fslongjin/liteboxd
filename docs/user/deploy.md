# LiteBoxd Deployment Guide (via liteboxd-installer)

本文说明如何通过 `liteboxd-installer` 一键部署 LiteBoxd。

## 1. 前置条件

- 控制机可以 SSH 登录 `master` 节点
- `master` 节点可以访问所有 `agent` 节点的 SSH 端口
- `agent` 节点可以是内网 IP（installer 会通过 `master` 跳转连接）
- 准备好各节点 root 密码（建议环境变量注入）

## 2. 构建 installer

在仓库根目录执行：

```bash
make build-installer
```

生成二进制：

```bash
bin/liteboxd-installer
```

## 3. 准备配置文件

推荐从示例复制一份再修改：

```bash
cp tools/liteboxd-installer/examples/install.example.yaml /tmp/liteboxd-install.yaml
```

至少需要按你的环境修改这些字段：

- `cluster.master.host` / `cluster.master.nodeIP`
- `cluster.agents[]`（可为空）
- `cluster.master.password` / `cluster.agents[].password`
- `liteboxd.ingressHost`
- `liteboxd.images.api/gateway/web`
- `liteboxd.configDir`（LiteBoxd kustomize patch 目录）

## 4. 设置密码环境变量

```bash
export MASTER_PASS='your-master-password'
export AGENT1_PASS='your-agent1-password'
export AGENT2_PASS='your-agent2-password'
```

## 5. 执行部署

```bash
./bin/liteboxd-installer -f /tmp/liteboxd-install.yaml apply
```

常用参数：

- `--log-file /tmp/liteboxd-installer.log`：输出详细执行日志
- `--state /tmp/liteboxd-state.json`：指定状态文件位置
- `--dry-run`：只打印计划，不真正执行
- `--cluster-only`：仅维护 K3s/Cilium/节点，不部署 LiteBoxd

示例：

```bash
./bin/liteboxd-installer \
  -f /tmp/liteboxd-install.yaml \
  --log-file /tmp/liteboxd-installer.log \
  apply
```

## 6. 失败重试

如果中断或失败，可直接重试：

```bash
./bin/liteboxd-installer -f /tmp/liteboxd-install.yaml resume
```

## 7. 扩缩容与节点移除

- 扩容：更新 `cluster.agents` 后重新执行 `apply`
- 删除节点：

```bash
./bin/liteboxd-installer \
  -f /tmp/liteboxd-install.yaml \
  node remove --hosts 10.10.10.13
```

## 8. 说明

- installer 为幂等设计，重复执行 `apply` 是预期用法
- Cilium 配置未变化时会跳过安装/升级步骤，避免不必要网络抖动
