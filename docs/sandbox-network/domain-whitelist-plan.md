# 出网域名白名单实现方案

## 1. 背景与现状

- 当前网络模型已有 `AllowedDomains` 字段，用于域名白名单功能
- 出网控制依赖 `internet-access=true` 标签与 `allow-internet-egress` NetworkPolicy
- 域名白名单通过 CiliumNetworkPolicy 实现，支持精确域名与通配符匹配

相关位置：
- [client.go](file:///home/longjin/code/lwsandbox/backend/internal/k8s/client.go#L82-L102)
- [network.go](file:///home/longjin/code/lwsandbox/backend/internal/model/network.go#L1-L14)
- [README.md](file:///home/longjin/code/lwsandbox/docs/sandbox-network/README.md#L112-L114)

## 2. 目标与非目标

目标：
- 支持模板级 `AllowedDomains` 白名单并在沙箱出网策略中生效
- 允许精确域名与通配符域名
- 对不在白名单内的外部访问进行拒绝
- **允许 `allowInternetAccess` 与 `allowedDomains` 独立配置，方便测试**

非目标：
- 不实现 TCP/UDP 非 HTTP(S) 白名单
- 不引入跨命名空间或多租户隔离改造
- 不在本阶段支持实时动态更新白名单

## 3. 方案概览

采用 Cilium FQDN egress 策略，通过 CiliumNetworkPolicy 为每个沙箱生成白名单策略：

- 仅对白名单域名开放 80/443
- 允许 DNS 解析到 kube-dns
- 与现有 `default-deny-all` 及 `allow-dns` 保持一致
- 当 `AllowedDomains` 为空且 `AllowInternetAccess=true` 时继续使用现有 `allow-internet-egress` 逻辑
- 当 `AllowedDomains` 非空时不设置 `internet-access=true` 标签，避免放开全量出网

## 4. 关键设计变更：独立配置模式

### 4.1 变更背景

**问题**：原先的设计强制要求当配置 `allowedDomains` 时，`allowInternetAccess` 必须为 `true`。这导致用户在测试时需要反复配置域名白名单。

**解决方案**：解耦两个配置项，允许独立控制。

### 4.2 新的行为模型

| allowInternetAccess | allowedDomains | 实际行为 |
|---------------------|----------------|----------|
| `false` | `[]` | 完全禁止公网访问 |
| `false` | `["a.com"]` | 完全禁止公网访问（域名白名单被保存但暂不生效） |
| `true` | `[]` | 允许所有公网访问（80/443端口） |
| `true` | `["a.com"]` | 仅允许访问白名单域名 |

### 4.3 前端联动逻辑变更

**修改前**：
- 关闭"允许公网访问" → 自动清空域名白名单
- 配置域名白名单 → 自动开启"允许公网访问"

**修改后**：
- 两者完全独立，无任何自动联动
- 用户可以在有域名白名单配置的情况下，通过快速切换"允许公网访问"开关来测试效果

## 5. 数据流与生效路径

1. 模板定义 `NetworkSpec.AllowedDomains`
2. 创建沙箱时将 `NetworkSpec` 传入 `CreatePodOptions`
3. 仅当 `AllowInternetAccess=true` 且 `AllowedDomains` 非空时，由网络策略管理器创建对应 CiliumNetworkPolicy
4. 沙箱删除时同步清理对应策略

## 6. 策略设计

### 6.1 CiliumNetworkPolicy 样例

```yaml
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: sandbox-egress-allowlist-{sandbox-id}
  namespace: liteboxd
spec:
  endpointSelector:
    matchLabels:
      app: liteboxd
      sandbox-id: "{sandbox-id}"
  egress:
    - toFQDNs:
        - matchName: "api.example.com"
        - matchPattern: "*.example.org"
      toPorts:
        - ports:
            - port: "443"
              protocol: TCP
          - port: "80"
              protocol: TCP
    - toEndpoints:
        - matchLabels:
            k8s-app: kube-dns
          matchExpressions:
            - key: io.kubernetes.pod.namespace
              operator: In
              values:
                - kube-system
      toPorts:
        - ports:
            - port: "53"
              protocol: UDP
          - port: "53"
              protocol: TCP
```

### 6.2 白名单规则

- 仅允许域名，不允许 scheme/path/端口
- 支持 `example.com` 精确匹配
- 支持 `*.example.com` 通配符（将映射为 Cilium `matchPattern`）
- 输入统一转小写并去掉尾部点

### 6.3 行为约定（更新后）

- `AllowInternetAccess=false` 且 `AllowedDomains` 为空：禁止所有公网访问
- `AllowInternetAccess=false` 且 `AllowedDomains` 非空：禁止所有公网访问（域名配置被保存但不生效）
- `AllowInternetAccess=true` 且 `AllowedDomains` 为空：允许所有公网访问（80/443端口）
- `AllowInternetAccess=true` 且 `AllowedDomains` 非空：仅按白名单放行

## 7. 代码改动清单

### 7.1 模型与校验

- `template.go`: 移除 `allowInternetAccess` 必须为 `true` 的验证限制
- 域名白名单配置独立存储，不依赖 `allowInternetAccess` 状态

### 7.2 网络策略管理

- `sandbox.go`: 仅当 `AllowInternetAccess=true` 且 `AllowedDomains` 非空时才应用 CiliumNetworkPolicy
- 新增 CiliumNetworkPolicy 创建/更新/删除逻辑
- 使用动态客户端或 Unstructured 资源调用
- 策略名称以 `sandbox-egress-allowlist-{sandbox-id}` 命名
- 沙箱删除流程中清理策略

### 7.3 标签与注解

- 保留 `internet-access=true` 语义用于全量出网
- 白名单模式下不打该标签，避免放开 0.0.0.0/0

### 7.4 前端联动

- `TemplateList.vue`: 移除 `networkAllowInternet` 与 `networkAllowedDomains` 之间的自动联动逻辑
- 更新 UI 提示文案，明确域名白名单需要开启公网访问才能生效

## 8. 测试与验证

### 8.1 单元测试

- `TestValidateNetworkSpecRequiresInternetAccess`: 验证 `allowInternetAccess=false` 时可以配置域名白名单
- `TestValidateNetworkSpecWithInternetAccessAndDomains`: 验证开启公网且有域名的场景
- `TestValidateNetworkSpecWithInternetAccessOnly`: 验证仅开启公网的场景
- `TestValidateNetworkSpecDisabled`: 验证完全禁用公网的场景
- `TestDomainAllowlistPolicyWithWildcard`: 验证通配符域名策略
- `TestDomainAllowlistPolicyWithEmptyDomains`: 验证空域名列表不创建策略

### 8.2 集成测试

- 仅白名单域名可访问
- 非白名单域名访问失败
- 空白名单且允许出网时仍可访问公网
- `allowInternetAccess=false` 时域名白名单不生效

## 9. 风险与注意事项

- Cilium FQDN 解析依赖 DNS，需确保 kube-dns egress 始终可用
- FQDN 缓存与 TTL 可能导致短时间策略延迟
- 若集群未安装 Cilium CRD，需要在部署步骤中显式声明依赖
- 用户需要理解 `allowInternetAccess=false` 时域名白名单配置会被保留但不生效

## 10. 里程碑拆分

1. ~~规则定义与参数校验~~ ✅ 已完成
2. ~~CiliumNetworkPolicy 生成与应用~~ ✅ 已完成
3. ~~沙箱删除时清理策略~~ ✅ 已完成
4. ~~独立配置模式实现~~ ✅ 已完成
5. ~~集成测试与文档更新~~ ✅ 已完成
