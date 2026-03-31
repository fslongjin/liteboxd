# LiteBoxd 沙箱文件描述符上限修复

## 方案摘要

- 平台统一为每个新建或重建后的 sandbox 注入 `sandbox-launcher`
- `sandbox-launcher` 是静态 Go 二进制，只做 `setrlimit(RLIMIT_NOFILE) + exec`
- 默认 `RLIMIT_NOFILE=16384`
- launcher 通过 initContainer 注入，不要求业务镜像内置任何脚本或工具
- launcher 只暴露在只读单用途路径 `/.liteboxd-injected/launcher/sandbox-launcher`

## 关键实现

1. `backend/cmd/sandbox-launcher`
   - 新增 launcher 二进制与参数解析测试
2. `backend/internal/k8s`
   - 普通 Pod 创建时注入 `launcher-init`、`sandbox-launcher` 卷和只读挂载
   - 持久化 Deployment 同样注入 launcher，并在 rootfs wrapper 最终执行前接入 launcher
   - `Exec` / `ExecInteractive` 路径统一通过 launcher 包装，避免绕过 fd 限制
3. `backend/cmd/server`
   - 新增 `SANDBOX_NOFILE_LIMIT` 与 `SANDBOX_LAUNCHER_IMAGE` 配置
4. 构建与部署
   - 新增 `backend/Dockerfile.launcher`
   - 构建脚本、installer 资产与示例配置新增 launcher 镜像配置

## 生效范围

- 仅对新建、重建、重启后的沙箱生效
- 已在运行的旧沙箱不会被主动收敛

## 回滚

- 将 `SANDBOX_NOFILE_LIMIT` 设为 `0` 即可关闭该能力
