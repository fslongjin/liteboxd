# Backend 鉴权方案设计

## 1. 背景

当前 lwsandbox 的 API Server（8080 端口）没有任何鉴权，所有人都可以直接调用控制面 API（创建/删除 sandbox、管理模板等）。需要引入鉴权机制保护 API 访问。

### 需求

- 支持一个 **admin 用户**（单管理员，非多租户）
- admin 可以创建 **API Key**，供 SDK/CLI 程序化访问
- Web 端使用**登录态（cookie session）** 鉴权
- Gateway Server（8081 端口）的 per-sandbox token 鉴权保持不变

## 2. 整体架构

```
                        ┌──────────────────────────────────┐
                        │          API Server :8080        │
                        │                                  │
  Web (Browser)  ───────┤  Cookie Session                  │
                        │    ↓                             │
                        │  Auth Middleware ──→ Handler      │
                        │    ↑                             │
  SDK/CLI  ─────────────┤  Bearer API Key                  │
                        │                                  │
                        │  Public: /health, /readyz,       │
                        │          /api/v1/auth/login      │
                        └──────────────────────────────────┘

                        ┌──────────────────────────────────┐
                        │       Gateway Server :8081       │
                        │                                  │
  Sandbox Client ───────┤  X-Access-Token（不变）           │
                        └──────────────────────────────────┘
```

### 鉴权方式

| 场景 | 鉴权方式 | 传输方式 |
|------|----------|----------|
| Web 端 | Server-side Session | `Set-Cookie: liteboxd_session=<token>` (HttpOnly) |
| SDK/CLI | API Key | `Authorization: Bearer lbxk_<hex>` |
| Sandbox 数据面 | Per-sandbox Token | `X-Access-Token: <token>`（不变） |

## 3. 数据库 Schema

在现有 SQLite 数据库中新增 3 张表。

### 3.1 admin_users — 管理员用户表

```sql
CREATE TABLE IF NOT EXISTS admin_users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

- `password_hash`：bcrypt 哈希（cost=12）
- 当前只支持一个 admin 用户，由服务启动时初始化

### 3.2 sessions — 登录会话表

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES admin_users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
```

- `id`：`SHA-256(session_token)` — 数据库只存哈希值，不存明文
- 过期的 session 由后台定时任务清理

### 3.3 api_keys — API Key 表

```sql
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(prefix);
```

- `name`：人类可读标签（如 "ci-pipeline"、"dev-machine"）
- `prefix`：key 的前 8 字符，用于在列表中辨识（如 `a1b2c3d4`）
- `key_hash`：`SHA-256(full_key)` — 明文不落盘
- `expires_at`：可选过期时间，NULL 表示永不过期
- `last_used_at`：最近使用时间，用于审计

## 4. Admin 用户初始化

### 环境变量

| 变量名 | 必填 | 默认值 | 说明 |
|--------|------|--------|------|
| `ADMIN_USERNAME` | 否 | `admin` | 管理员用户名 |
| `ADMIN_PASSWORD` | 否 | `liteboxd-admin` | 管理员密码 |

### 初始化逻辑

服务启动时（`store.InitDB()` 之后、HTTP server 启动之前）执行：

```
IF admin_users 表为空:
    创建 admin 用户（使用 ADMIN_PASSWORD 或默认密码 "liteboxd-admin"，bcrypt hash）
ELSE IF ADMIN_PASSWORD 已设置且非默认值:
    更新已有 admin 用户的密码（允许通过重启服务来重置密码）
```

### 实现位置

新建 `backend/internal/auth/admin.go`，提供 `EnsureAdmin(ctx)` 函数。

## 5. Session 管理（Web 端鉴权）

### 为什么选择 Server-side Session 而非 JWT

| | Server-side Session | JWT |
|---|---|---|
| 即时失效 | 支持（删除 DB 记录即可） | 不支持（需维护黑名单） |
| 存储 | SQLite（已有） | 无需额外存储 |
| 适用场景 | 单机、单用户 | 分布式、多服务 |
| 实现复杂度 | 低 | 中（signing key 管理） |

lwsandbox 是单机 SQLite 架构，Session 方案更合适。

### 登录流程

```
POST /api/v1/auth/login
Body: {"username": "admin", "password": "***"}

  1. 查 admin_users 表获取 password_hash
  2. bcrypt.CompareHashAndPassword 验证密码
  3. 生成 32 字节随机 token（复用 security.GenerateToken(32)）
  4. SHA-256(token) 存入 sessions 表，设置过期时间（默认 24 小时）
  5. Set-Cookie: liteboxd_session=<token>; Path=/; HttpOnly; SameSite=Lax; Max-Age=86400
     （HTTPS 环境下加 Secure 标志）
  6. 返回 {"message": "login successful", "username": "admin"}
```

### 登出流程

```
POST /api/v1/auth/logout
Cookie: liteboxd_session=<token>

  1. SHA-256(cookie_token) 查 sessions 表
  2. 删除对应 session 记录
  3. Set-Cookie: liteboxd_session=; Max-Age=0  （清除 cookie）
  4. 返回 {"message": "logout successful"}
```

### Session 清理

后台 goroutine 每小时运行一次，删除所有 `expires_at < now()` 的 session 记录。复用现有的 `StartTTLCleaner` / `StartMetadataCleaner` 的 ticker 模式。

### Session 有效期配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `SESSION_MAX_AGE` | `86400`（24小时） | Session 有效期（秒） |

## 6. API Key 管理

### Key 格式

```
lbxk_<64位hex字符>
```

- 前缀 `lbxk_`（liteboxd key）便于识别和 secret scanner 检测
- 64 位 hex = 32 字节随机数，与现有 sandbox token 格式一致

### 创建流程

```
POST /api/v1/auth/api-keys  （需要 session 登录）
Body: {"name": "my-ci-key", "expires_in_days": 90}  // expires_in_days 可选

  1. security.GenerateToken(32) 生成随机 hex
  2. 拼接前缀得到 full_key = "lbxk_" + hex
  3. 计算 key_hash = security.HashToken(full_key)
  4. 提取 prefix = hex[:8]
  5. 存入 api_keys 表
  6. 返回 full_key 明文（仅此一次！）
```

响应：
```json
{
    "id": "uuid-xxx",
    "name": "my-ci-key",
    "key": "lbxk_a1b2c3d4...",
    "prefix": "a1b2c3d4",
    "expires_at": "2026-05-27T00:00:00Z",
    "created_at": "2026-02-26T00:00:00Z"
}
```

### 列出 API Keys

```
GET /api/v1/auth/api-keys  （需要 session 登录）
```

返回所有 key 的元数据（不含明文和 hash）：

```json
[
    {
        "id": "uuid-xxx",
        "name": "my-ci-key",
        "prefix": "a1b2c3d4",
        "expires_at": "2026-05-27T00:00:00Z",
        "last_used_at": "2026-02-26T12:00:00Z",
        "created_at": "2026-02-26T00:00:00Z"
    }
]
```

### 删除 API Key

```
DELETE /api/v1/auth/api-keys/:id  （需要 session 登录）
```

直接从 DB 删除记录，该 key 立即失效。

### API Key 管理权限

API Key 的创建/列出/删除**仅限 session 鉴权**（即必须通过 Web 端登录后操作）。不允许用 API Key 创建新的 API Key，避免 key 泄露后无限增殖。

## 7. Auth Middleware 设计

### 鉴权流程

```
请求进入
    │
    ├─ 有 Authorization: Bearer <token> ?
    │   ├─ YES → key_hash = SHA-256(token)
    │   │        查 api_keys 表
    │   │        ├─ 找到且未过期 → 认证通过（auth_method = "api_key"）
    │   │        │                  异步更新 last_used_at
    │   │        └─ 未找到或已过期 → 401 Unauthorized
    │   │
    │   └─ NO → 有 liteboxd_session cookie ?
    │       ├─ YES → token_hash = SHA-256(cookie)
    │       │        查 sessions 表
    │       │        ├─ 找到且未过期 → 认证通过（auth_method = "session"）
    │       │        └─ 未找到或已过期 → 401 Unauthorized
    │       │
    │       └─ NO → 401 Unauthorized
```

### 关键行为

1. **Bearer token 优先**：如果请求带了 `Authorization: Bearer` 头，仅尝试 API Key 鉴权。失败时直接返回 401，不 fallback 到 cookie。这避免了混淆。
2. **Context 注入**：认证通过后，将 `auth_method`（"session" / "api_key"）和 `user_id`（仅 session 时有）写入 Gin Context，供 handler 使用。
3. **异步更新 last_used_at**：API Key 的最后使用时间通过 goroutine 异步更新，不阻塞请求。

### 复用现有安全模块

- `security.HashToken(token string) string` — SHA-256 哈希
- `security.GenerateToken(size int) (string, error)` — 随机 token 生成

无需引入新的加密/哈希库。

### 免鉴权路由

以下路由不经过 Auth Middleware：

| 路由 | 说明 |
|------|------|
| `GET /health` | 健康检查 |
| `GET /readyz` | 就绪探针 |
| `POST /api/v1/auth/login` | 登录 |

## 8. API 端点汇总

### 新增端点

| 方法 | 路径 | 鉴权要求 | 说明 |
|------|------|----------|------|
| `POST` | `/api/v1/auth/login` | 无 | 用户名密码登录 |
| `POST` | `/api/v1/auth/logout` | Session | 登出（清除 session） |
| `GET` | `/api/v1/auth/me` | Session 或 API Key | 获取当前认证信息 |
| `POST` | `/api/v1/auth/api-keys` | Session | 创建 API Key |
| `GET` | `/api/v1/auth/api-keys` | Session | 列出所有 API Key |
| `DELETE` | `/api/v1/auth/api-keys/:id` | Session | 删除指定 API Key |

### 路由组织

```go
// 公开路由
r.GET("/health", healthHandler)
r.GET("/readyz", readyzHandler)

// Auth 路由（login 公开，其余需鉴权）
authGroup := r.Group("/api/v1/auth")
authGroup.POST("/login", authHandler.Login)
authProtected := authGroup.Group("")
authProtected.Use(authMiddleware)
{
    authProtected.POST("/logout", authHandler.Logout)
    authProtected.GET("/me", authHandler.Me)
    authProtected.POST("/api-keys", authHandler.CreateAPIKey)
    authProtected.GET("/api-keys", authHandler.ListAPIKeys)
    authProtected.DELETE("/api-keys/:id", authHandler.DeleteAPIKey)
}

// 业务 API（全部需鉴权）
api := r.Group("/api/v1")
api.Use(authMiddleware)
sandboxHandler.RegisterRoutes(api)
templateHandler.RegisterRoutes(api)
prepullHandler.RegisterRoutes(api)
importExportHandler.RegisterRoutes(api)
```

## 9. 安全考量

### 9.1 密码安全

- **bcrypt cost=12**：在安全性和性能之间取得平衡
- 密码明文仅在登录请求中传输，不存储、不日志记录
- 支持通过重启服务 + 设置 `ADMIN_PASSWORD` 环境变量重置密码

### 9.2 Token 安全

- Session token 和 API Key 均使用 `crypto/rand` 生成（32 字节 = 256 bit 熵）
- 数据库仅存 SHA-256 哈希，即使数据库泄露也无法还原明文
- API Key 明文仅在创建时返回一次

### 9.3 Cookie 安全

| 属性 | 值 | 作用 |
|------|-----|------|
| `HttpOnly` | `true` | 防止 JavaScript 读取 cookie（XSS 防护） |
| `SameSite` | `Lax` | 防止跨站 POST 请求携带 cookie（CSRF 防护），同时允许正常的链接跳转 |
| `Secure` | HTTPS 下 `true` | 防止 cookie 在 HTTP 明文中传输 |
| `Path` | `/` | cookie 对所有 API 路径可用 |
| `Max-Age` | `86400` | 与服务端 session 过期时间一致 |

### 9.4 CSRF 防护

`SameSite=Lax` + `HttpOnly` cookie + JSON Content-Type 的组合已足够防护 CSRF：
- `SameSite=Lax` 阻止浏览器在跨站 POST/DELETE 请求中携带 cookie（同时允许正常的链接跳转）
- API 使用 JSON body（非表单），无法通过 `<form>` 提交触发
- 不需要额外的 CSRF token

### 9.5 CORS 配置

使用 `AllowOriginFunc` 允许所有来源（动态回显请求的 Origin），配合 `AllowCredentials: true` 使 cookie 正常工作。CSRF 防护依赖 cookie 的 `SameSite` 属性而非 CORS 限制。

### 9.6 API Key 安全

- `lbxk_` 前缀便于 secret scanner（如 GitHub Secret Scanning）识别泄露的 key
- 支持过期时间，建议用户为 CI/CD key 设置合理过期期
- `last_used_at` 字段便于审计异常使用
- 仅 session 登录可管理 key，API Key 不能创建新 key（防止泄露后增殖）

## 10. SDK / CLI 兼容性

| 组件 | 现有支持 | 是否需要改动 |
|------|----------|-------------|
| Go SDK | `WithAuthToken(token)` → `Authorization: Bearer <token>` | 无需改动 |
| Python SDK | `auth_token` 参数 → `Authorization: Bearer <token>` | 无需改动 |
| CLI | config profile 的 `token` 字段 | 无需改动 |

用户只需将创建的 API Key 配置到 SDK/CLI 即可：

```go
// Go SDK
client := liteboxd.NewClient(
    "http://localhost:8080/api/v1",
    liteboxd.WithAuthToken("lbxk_a1b2c3d4..."),
)
```

```python
# Python SDK
client = LiteboxdClient(
    base_url="http://localhost:8080/api/v1",
    auth_token="lbxk_a1b2c3d4...",
)
```

```yaml
# CLI config (~/.config/liteboxd/config.yaml)
profiles:
  production:
    api-server: https://api.example.com/api/v1
    token: lbxk_a1b2c3d4...
```

## 11. 前端变更

### 新增页面

| 页面 | 路径 | 说明 |
|------|------|------|
| Login | `/login` | 登录页面 |
| API Keys | `/settings/api-keys` | API Key 管理页面 |

### 前端 Auth API Client

新建 `web/src/api/auth.ts`，提供登录、登出、获取用户信息、API Key CRUD 接口。

### Axios 配置变更

所有 axios 实例需添加 `withCredentials: true`，确保浏览器自动携带 cookie：

```typescript
const api = axios.create({
    baseURL: import.meta.env.VITE_API_URL || '/api/v1',
    timeout: 30000,
    withCredentials: true,  // 新增
})
```

涉及文件：`web/src/api/sandbox.ts`、`web/src/api/template.ts`

### 路由守卫

在 Vue Router 添加全局前置守卫，未登录用户自动跳转到 `/login`：

```typescript
router.beforeEach(async (to, from, next) => {
    if (to.meta.public) return next()
    try {
        await authApi.me()
        next()
    } catch {
        next({ name: 'login', query: { redirect: to.fullPath } })
    }
})
```

### Header 用户菜单

在 `App.vue` 的 Header 中添加用户下拉菜单，包含：
- 当前用户名显示
- "API Keys" 链接
- "登出" 按钮

## 12. 配置项汇总

| 环境变量 | 必填 | 默认值 | 说明 |
|----------|------|--------|------|
| `ADMIN_USERNAME` | 否 | `admin` | 管理员用户名 |
| `ADMIN_PASSWORD` | 否 | `liteboxd-admin` | 管理员密码（设置后也可用于重置密码） |
| `SESSION_MAX_AGE` | 否 | `86400` | Session 有效期（秒） |

## 13. 代码变更清单

### 新建文件

| 文件 | 用途 |
|------|------|
| `backend/internal/auth/admin.go` | Admin 初始化逻辑（EnsureAdmin） |
| `backend/internal/auth/middleware.go` | Auth Middleware（session + API key 双模式） |
| `backend/internal/store/auth.go` | AuthStore（admin_users / sessions / api_keys CRUD） |
| `backend/internal/handler/auth.go` | Auth HTTP handlers（login/logout/me/api-key CRUD） |
| `web/src/api/auth.ts` | 前端 Auth API client |
| `web/src/views/Login.vue` | 登录页面 |
| `web/src/views/APIKeys.vue` | API Key 管理页面 |

### 修改文件

| 文件 | 改动 |
|------|------|
| `backend/internal/store/sqlite.go` | `createTables()` 新增 3 张表及索引 |
| `backend/cmd/server/main.go` | 接入 auth 全流程：EnsureAdmin、AuthStore、AuthHandler、Middleware 注册、Session 清理定时任务、CORS 配置 |
| `backend/go.mod` | 提升 `golang.org/x/crypto` 为直接依赖（bcrypt） |
| `web/src/api/sandbox.ts` | axios 添加 `withCredentials: true` |
| `web/src/api/template.ts` | axios 添加 `withCredentials: true` |
| `web/src/router/index.ts` | 新增 login/api-keys 路由 + 导航守卫 |
| `web/src/App.vue` | Header 新增用户菜单 |

## 14. 实施步骤

1. **Phase 1 — Backend 数据层**：新增 3 张表（sqlite.go）+ AuthStore CRUD（store/auth.go）+ Admin 初始化（auth/admin.go）
2. **Phase 2 — Backend Middleware + Handler**：Auth Middleware（auth/middleware.go）+ Auth Handler（handler/auth.go）+ main.go 接入
3. **Phase 3 — 前端**：Auth API client + Login 页面 + API Keys 页面 + 路由守卫 + withCredentials
4. **Phase 4 — 测试与文档**：端到端测试、更新 env.example、更新 OpenAPI spec
