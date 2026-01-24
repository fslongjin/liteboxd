# 轻量级沙箱模版系统设计方案

## 1. 概述

### 1.1 背景

当前 LiteBoxd 系统每次创建沙箱都需要指定完整的配置（镜像、CPU、内存、环境变量等），缺乏模版复用能力。参考 [E2B](https://e2b.dev/docs/sandbox-template) 的设计理念，设计一套轻量级的沙箱模版系统。

### 1.2 设计目标

- **简单易用**: 通过 YAML 文件定义模版，无需额外的构建步骤
- **快速实例化**: 从模版创建沙箱只需指定模版 ID
- **版本管理**: 支持模版的版本控制和回滚
- **可扩展**: 支持自定义启动脚本、文件预置等高级功能

### 1.3 与 E2B 的对比

| 特性 | E2B | LiteBoxd (本方案) |
|------|-----|------------------|
| 模版定义 | Dockerfile | YAML 配置文件 |
| 底层技术 | Firecracker microVM | Kubernetes Pod |
| 构建方式 | 镜像构建 + 快照 | 配置预设（无构建） |
| 启动速度 | <125ms | 取决于镜像拉取 |
| 隔离级别 | 硬件级 (KVM) | 容器级 (cgroups) |

## 2. 核心概念

### 2.1 模版 (Template)

模版是一组预定义的沙箱配置，包含：

- **基础镜像**: 容器镜像地址
- **资源配置**: CPU、内存限制
- **环境变量**: 预设的环境变量
- **启动命令**: 容器启动时执行的命令
- **预置文件**: 启动后自动上传的文件
- **生命周期**: 默认 TTL 等

### 2.2 模版版本 (Template Version)

每个模版可以有多个版本：
- 每次更新模版配置会创建新版本
- 支持指定版本创建沙箱
- 支持回滚到历史版本

### 2.3 实例化 (Instantiation)

从模版创建沙箱的过程：
1. 读取模版配置
2. 合并用户覆盖参数
3. 创建 Pod
4. 执行启动脚本
5. 上传预置文件

## 3. 数据模型

### 3.1 模版定义 (template.yaml)

```yaml
# 模版元信息
apiVersion: liteboxd/v1
kind: SandboxTemplate
metadata:
  name: python-data-science    # 模版唯一标识
  displayName: Python 数据科学环境
  description: 预装 numpy, pandas, matplotlib 的 Python 环境
  tags:
    - python
    - data-science
  author: admin

# 模版规格
spec:
  # 基础镜像 (必填)
  image: python:3.11-slim

  # 资源配置 (可选，有默认值)
  resources:
    cpu: "500m"        # 默认 500m
    memory: "512Mi"    # 默认 512Mi

  # 默认 TTL (可选)
  ttl: 3600            # 默认 3600 秒

  # 环境变量 (可选)
  env:
    PYTHONUNBUFFERED: "1"
    PIP_NO_CACHE_DIR: "1"

  # 启动命令 (可选)
  # 容器启动后执行，用于安装依赖或初始化环境
  startupScript: |
    pip install numpy pandas matplotlib scikit-learn
    echo "Environment ready!"

  # 启动命令超时 (可选)
  startupTimeout: 300  # 秒，默认 300

  # 预置文件 (可选)
  # 启动后自动上传到沙箱的文件
  files:
    - source: ./templates/python-data-science/setup.py
      destination: /workspace/setup.py
    - source: ./templates/python-data-science/examples/
      destination: /workspace/examples/

  # 就绪检查 (可选)
  readinessProbe:
    exec:
      command: ["python", "-c", "import numpy; print('ready')"]
    initialDelaySeconds: 5
    periodSeconds: 2
    failureThreshold: 30
```

### 3.2 数据库模型

```go
// Template 模版主表
type Template struct {
    ID          string    `json:"id"`          // UUID
    Name        string    `json:"name"`        // 唯一标识 (如 python-data-science)
    DisplayName string    `json:"displayName"` // 显示名称
    Description string    `json:"description"` // 描述
    Tags        []string  `json:"tags"`        // 标签
    Author      string    `json:"author"`      // 作者
    IsPublic    bool      `json:"isPublic"`    // 是否公开
    LatestVer   int       `json:"latestVersion"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}

// TemplateVersion 模版版本表
type TemplateVersion struct {
    ID          string    `json:"id"`
    TemplateID  string    `json:"templateId"`
    Version     int       `json:"version"`     // 版本号 (1, 2, 3...)
    Spec        TemplateSpec `json:"spec"`     // 完整规格配置
    Changelog   string    `json:"changelog"`   // 变更说明
    CreatedAt   time.Time `json:"createdAt"`
    CreatedBy   string    `json:"createdBy"`
}

// TemplateSpec 模版规格
type TemplateSpec struct {
    Image          string            `json:"image"`
    Resources      ResourceSpec      `json:"resources"`
    TTL            int               `json:"ttl"`
    Env            map[string]string `json:"env"`
    StartupScript  string            `json:"startupScript"`
    StartupTimeout int               `json:"startupTimeout"`
    Files          []FileSpec        `json:"files"`
    ReadinessProbe *ProbeSpec        `json:"readinessProbe,omitempty"`
}

// ResourceSpec 资源规格
type ResourceSpec struct {
    CPU    string `json:"cpu"`
    Memory string `json:"memory"`
}

// FileSpec 文件规格
type FileSpec struct {
    Source      string `json:"source"`      // 本地文件路径或内联内容
    Destination string `json:"destination"` // 沙箱内目标路径
    Content     string `json:"content"`     // 内联内容 (与 Source 二选一)
}

// ProbeSpec 就绪探针规格
type ProbeSpec struct {
    Exec                ExecAction `json:"exec"`
    InitialDelaySeconds int        `json:"initialDelaySeconds"`
    PeriodSeconds       int        `json:"periodSeconds"`
    FailureThreshold    int        `json:"failureThreshold"`
}
```

## 4. API 设计

### 4.1 模版管理 API

```yaml
# 模版 CRUD
POST   /api/v1/templates              # 创建模版
GET    /api/v1/templates              # 列出模版
GET    /api/v1/templates/{name}       # 获取模版详情
PUT    /api/v1/templates/{name}       # 更新模版 (创建新版本)
DELETE /api/v1/templates/{name}       # 删除模版

# 版本管理
GET    /api/v1/templates/{name}/versions           # 列出所有版本
GET    /api/v1/templates/{name}/versions/{version} # 获取特定版本
POST   /api/v1/templates/{name}/rollback           # 回滚到指定版本

# 模版文件
POST   /api/v1/templates/{name}/files    # 上传模版文件
GET    /api/v1/templates/{name}/files    # 列出模版文件
DELETE /api/v1/templates/{name}/files    # 删除模版文件
```

### 4.2 从模版创建沙箱

```yaml
# 扩展现有的创建沙箱 API
POST /api/v1/sandboxes

# 方式一：直接指定配置 (现有方式)
{
  "image": "python:3.11-slim",
  "cpu": "500m",
  "memory": "512Mi"
}

# 方式二：从模版创建 (新增)
{
  "template": "python-data-science",  # 模版名称
  "templateVersion": 2,               # 可选，默认使用最新版本
  "overrides": {                      # 可选，覆盖模版配置
    "cpu": "1000m",
    "memory": "1Gi",
    "env": {
      "DEBUG": "true"
    }
  }
}
```

### 4.3 API 请求/响应示例

#### 创建模版
```bash
POST /api/v1/templates
Content-Type: application/json

{
  "name": "python-data-science",
  "displayName": "Python 数据科学环境",
  "description": "预装数据科学工具包的 Python 环境",
  "tags": ["python", "data-science"],
  "spec": {
    "image": "python:3.11-slim",
    "resources": {
      "cpu": "500m",
      "memory": "512Mi"
    },
    "ttl": 3600,
    "env": {
      "PYTHONUNBUFFERED": "1"
    },
    "startupScript": "pip install numpy pandas matplotlib"
  }
}
```

#### 响应
```json
{
  "id": "tpl-abc123",
  "name": "python-data-science",
  "displayName": "Python 数据科学环境",
  "description": "预装数据科学工具包的 Python 环境",
  "tags": ["python", "data-science"],
  "latestVersion": 1,
  "createdAt": "2025-01-24T10:00:00Z"
}
```

## 5. 系统架构

### 5.1 组件架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Web Frontend                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ 模版列表页   │  │ 模版详情页   │  │ 从模版创建沙箱页面  │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        Backend API                           │
│  ┌─────────────────────────────────────────────────────────┐│
│  │                    handler/template.go                   ││
│  │         (模版 CRUD, 版本管理, 文件上传)                    ││
│  └─────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────┐│
│  │                   service/template.go                    ││
│  │     (模版业务逻辑, 版本控制, 配置验证, 文件管理)            ││
│  └─────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────┐  ┌────────────────────────────┐│
│  │  service/sandbox.go     │  │     store/template.go      ││
│  │  (扩展: 从模版创建沙箱)   │  │    (模版数据持久化)         ││
│  └─────────────────────────┘  └────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          ▼                   ▼                   ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│   Kubernetes    │  │  SQLite/PostgreSQL│  │   File Storage  │
│   (Pod 创建)    │  │   (模版元数据)    │  │  (模版文件)     │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

### 5.2 从模版创建沙箱流程

```
用户请求 ─────────────────────────────────────────────────────────────────┐
                                                                          │
┌─────────────────────────────────────────────────────────────────────────▼─┐
│ 1. 解析请求                                                               │
│    - 检查是否指定 template 字段                                            │
│    - 如果是直接配置，走原有流程                                             │
└─────────────────────────────────────────────────────────────────────────┬─┘
                                                                          │
┌─────────────────────────────────────────────────────────────────────────▼─┐
│ 2. 加载模版                                                               │
│    - 根据 template name 查询模版                                          │
│    - 获取指定版本或最新版本的 spec                                          │
└─────────────────────────────────────────────────────────────────────────┬─┘
                                                                          │
┌─────────────────────────────────────────────────────────────────────────▼─┐
│ 3. 合并配置                                                               │
│    - 以模版 spec 为基础                                                    │
│    - 应用 overrides 覆盖字段                                               │
│    - 合并环境变量 (覆盖同名，保留不同名)                                      │
└─────────────────────────────────────────────────────────────────────────┬─┘
                                                                          │
┌─────────────────────────────────────────────────────────────────────────▼─┐
│ 4. 创建 Pod                                                               │
│    - 使用合并后的配置创建 K8s Pod                                           │
│    - 等待 Pod 进入 Running 状态                                            │
└─────────────────────────────────────────────────────────────────────────┬─┘
                                                                          │
┌─────────────────────────────────────────────────────────────────────────▼─┐
│ 5. 执行初始化 (如果配置了 startupScript)                                   │
│    - 在 Pod 内执行 startupScript                                          │
│    - 等待脚本完成或超时                                                     │
└─────────────────────────────────────────────────────────────────────────┬─┘
                                                                          │
┌─────────────────────────────────────────────────────────────────────────▼─┐
│ 6. 上传预置文件 (如果配置了 files)                                         │
│    - 遍历 files 列表                                                       │
│    - 上传文件到沙箱指定路径                                                  │
└─────────────────────────────────────────────────────────────────────────┬─┘
                                                                          │
┌─────────────────────────────────────────────────────────────────────────▼─┐
│ 7. 就绪检查 (如果配置了 readinessProbe)                                    │
│    - 执行探针命令                                                          │
│    - 等待成功或超过重试次数                                                  │
└─────────────────────────────────────────────────────────────────────────┬─┘
                                                                          │
┌─────────────────────────────────────────────────────────────────────────▼─┐
│ 8. 返回结果                                                               │
│    - 返回 Sandbox 对象                                                     │
│    - 包含 templateName, templateVersion 字段                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 6. 存储方案

### 6.1 数据存储

**方案选择**: SQLite (轻量级) 或 PostgreSQL (生产环境)

```
选择 SQLite 的理由:
- 轻量级，无需额外部署数据库服务
- 单文件存储，便于备份和迁移
- 对于中小规模模版管理足够使用
```

**数据库表结构**:

```sql
-- 模版主表
CREATE TABLE templates (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    display_name TEXT,
    description TEXT,
    tags TEXT,           -- JSON 数组
    author TEXT,
    is_public BOOLEAN DEFAULT true,
    latest_version INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 模版版本表
CREATE TABLE template_versions (
    id TEXT PRIMARY KEY,
    template_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    spec TEXT NOT NULL,   -- JSON 格式的 TemplateSpec
    changelog TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by TEXT,
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,
    UNIQUE(template_id, version)
);

-- 索引
CREATE INDEX idx_templates_name ON templates(name);
CREATE INDEX idx_template_versions_template_id ON template_versions(template_id);
```

### 6.2 文件存储

模版关联的文件存储在本地文件系统:

```
data/
└── templates/
    └── {template-name}/
        ├── v1/
        │   ├── setup.py
        │   └── examples/
        │       └── demo.py
        └── v2/
            ├── setup.py
            └── examples/
                ├── demo.py
                └── advanced.py
```

## 7. 实现计划

### Phase 1: 核心模版功能

**目标**: 实现基础的模版 CRUD 和从模版创建沙箱

**后端改动**:
- [ ] 新增 `internal/model/template.go` - 模版数据模型
- [ ] 新增 `internal/store/template.go` - 模版数据访问层 (SQLite)
- [ ] 新增 `internal/service/template.go` - 模版业务逻辑
- [ ] 新增 `internal/handler/template.go` - 模版 API 处理器
- [ ] 修改 `internal/service/sandbox.go` - 支持从模版创建
- [ ] 修改 `internal/model/sandbox.go` - 添加模版相关字段
- [ ] 修改 `cmd/server/main.go` - 初始化模版服务

**前端改动**:
- [ ] 新增 `src/api/template.ts` - 模版 API 客户端
- [ ] 新增 `src/views/TemplateList.vue` - 模版列表页
- [ ] 新增 `src/views/TemplateDetail.vue` - 模版详情页
- [ ] 修改 `src/views/SandboxList.vue` - 支持从模版创建

### Phase 2: 高级功能

**目标**: 实现启动脚本、预置文件、就绪探针

**改动**:
- [ ] 实现 startupScript 执行逻辑
- [ ] 实现文件预置上传逻辑
- [ ] 实现 readinessProbe 检查逻辑
- [ ] 添加初始化进度反馈

### Phase 3: 版本管理

**目标**: 完善版本控制功能

**改动**:
- [ ] 实现版本列表 API
- [ ] 实现版本回滚 API
- [ ] 前端版本管理界面
- [ ] 版本对比功能

### Phase 4: 内置模版

**目标**: 提供开箱即用的常用模版

**内置模版列表**:
- [ ] `base` - 基础 Alpine 环境
- [ ] `python` - Python 3.11 环境
- [ ] `python-data-science` - 数据科学环境
- [ ] `nodejs` - Node.js 20 环境
- [ ] `golang` - Go 1.21 环境

## 8. 配置项

### 8.1 服务配置

```yaml
# config.yaml
template:
  # 存储配置
  storage:
    # 数据库类型: sqlite | postgres
    type: sqlite
    # SQLite 数据库文件路径
    sqlitePath: ./data/liteboxd.db
    # PostgreSQL 连接字符串 (当 type=postgres 时)
    postgresURL: ""

  # 文件存储路径
  filesPath: ./data/templates

  # 默认值
  defaults:
    cpu: "500m"
    memory: "512Mi"
    ttl: 3600
    startupTimeout: 300

  # 限制
  limits:
    maxTemplates: 100        # 最大模版数量
    maxVersions: 50          # 每个模版最大版本数
    maxFileSize: 10485760    # 单文件最大 10MB
    maxTotalFileSize: 104857600  # 模版文件总大小最大 100MB
```

## 9. 安全考虑

1. **输入验证**: 严格验证模版名称、镜像地址等输入
2. **权限控制**: 预留模版访问控制扩展点 (isPublic 字段)
3. **资源限制**: 限制模版数量、文件大小防止滥用
4. **镜像白名单**: 可配置允许使用的镜像前缀
5. **脚本执行**: startupScript 在沙箱内执行，不影响宿主机

## 10. 镜像预拉取功能

### 10.1 功能说明

镜像预拉取 (Image Pre-pull) 用于在创建沙箱之前预先将容器镜像拉取到 K8s 节点上，避免首次创建沙箱时因镜像拉取导致的长时间等待。

### 10.2 实现方式

使用 K8s DaemonSet 在所有节点上预拉取镜像：

```yaml
# 为每个需要预拉取的镜像创建一个 DaemonSet
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: image-prepull-{image-hash}
  namespace: liteboxd
  labels:
    app: image-prepull
    image-hash: {image-hash}
spec:
  selector:
    matchLabels:
      app: image-prepull
      image-hash: {image-hash}
  template:
    metadata:
      labels:
        app: image-prepull
        image-hash: {image-hash}
    spec:
      initContainers:
        - name: prepull
          image: {target-image}
          command: ["sh", "-c", "echo 'Image pulled successfully'"]
          resources:
            limits:
              cpu: "100m"
              memory: "64Mi"
      containers:
        - name: pause
          image: gcr.io/google_containers/pause:3.2
          resources:
            limits:
              cpu: "10m"
              memory: "16Mi"
      tolerations:
        - operator: Exists  # 在所有节点运行
```

### 10.3 API 设计

```yaml
# 镜像预拉取 API
POST   /api/v1/images/prepull           # 触发镜像预拉取
GET    /api/v1/images/prepull           # 查询预拉取状态
DELETE /api/v1/images/prepull/{image}   # 取消/删除预拉取任务

# 模版自动预拉取
POST   /api/v1/templates/{name}/prepull # 预拉取模版使用的镜像
```

**触发预拉取请求**:
```json
POST /api/v1/images/prepull
{
  "image": "python:3.11-slim",
  "timeout": 600  // 可选，超时秒数
}
```

**查询预拉取状态响应**:
```json
{
  "items": [
    {
      "image": "python:3.11-slim",
      "status": "completed",  // pending | pulling | completed | failed
      "progress": {
        "ready": 3,
        "total": 3
      },
      "startedAt": "2025-01-24T10:00:00Z",
      "completedAt": "2025-01-24T10:02:30Z"
    }
  ]
}
```

### 10.4 数据模型

```go
// ImagePrepull 镜像预拉取记录
type ImagePrepull struct {
    ID          string    `json:"id"`
    Image       string    `json:"image"`
    ImageHash   string    `json:"imageHash"`   // 镜像名称的 hash，用于命名
    Status      string    `json:"status"`      // pending | pulling | completed | failed
    ReadyNodes  int       `json:"readyNodes"`  // 已就绪节点数
    TotalNodes  int       `json:"totalNodes"`  // 总节点数
    Error       string    `json:"error,omitempty"`
    StartedAt   time.Time `json:"startedAt"`
    CompletedAt time.Time `json:"completedAt,omitempty"`
}
```

### 10.5 自动预拉取

创建模版时可选择自动预拉取：

```json
POST /api/v1/templates
{
  "name": "python-data-science",
  "spec": {
    "image": "python:3.11-slim",
    ...
  },
  "autoPrepull": true  // 创建模版后自动预拉取镜像
}
```

## 11. YAML 批量导入功能

### 11.1 功能说明

支持通过 YAML 文件批量导入模版，便于模版的版本控制、迁移和共享。

### 11.2 YAML 文件格式

**单个模版文件** (`python-data-science.yaml`):
```yaml
apiVersion: liteboxd/v1
kind: SandboxTemplate
metadata:
  name: python-data-science
  displayName: Python 数据科学环境
  description: 预装 numpy, pandas, matplotlib 的 Python 环境
  tags:
    - python
    - data-science
spec:
  image: python:3.11-slim
  resources:
    cpu: "500m"
    memory: "512Mi"
  ttl: 3600
  env:
    PYTHONUNBUFFERED: "1"
  startupScript: |
    pip install numpy pandas matplotlib
```

**批量模版文件** (`templates.yaml`):
```yaml
apiVersion: liteboxd/v1
kind: SandboxTemplateList
items:
  - metadata:
      name: python
      displayName: Python 3.11
    spec:
      image: python:3.11-slim
      resources:
        cpu: "500m"
        memory: "512Mi"

  - metadata:
      name: nodejs
      displayName: Node.js 20
    spec:
      image: node:20-slim
      resources:
        cpu: "500m"
        memory: "512Mi"

  - metadata:
      name: golang
      displayName: Go 1.21
    spec:
      image: golang:1.21-alpine
      resources:
        cpu: "1000m"
        memory: "1Gi"
```

### 11.3 API 设计

```yaml
# YAML 导入导出 API
POST /api/v1/templates/import    # 导入 YAML 文件
GET  /api/v1/templates/export    # 导出所有模版为 YAML
GET  /api/v1/templates/{name}/export  # 导出单个模版为 YAML
```

**导入请求**:
```bash
POST /api/v1/templates/import
Content-Type: multipart/form-data

file: @templates.yaml
strategy: "create-or-update"  # create-only | update-only | create-or-update
prepull: true                  # 是否自动预拉取镜像
```

**导入响应**:
```json
{
  "total": 3,
  "created": 2,
  "updated": 1,
  "skipped": 0,
  "failed": 0,
  "results": [
    {"name": "python", "action": "created", "version": 1},
    {"name": "nodejs", "action": "created", "version": 1},
    {"name": "golang", "action": "updated", "version": 2}
  ],
  "prepullStarted": ["python:3.11-slim", "node:20-slim", "golang:1.21-alpine"]
}
```

**导出响应** (YAML):
```yaml
# GET /api/v1/templates/export
apiVersion: liteboxd/v1
kind: SandboxTemplateList
exportedAt: "2025-01-24T10:00:00Z"
items:
  - metadata:
      name: python
      ...
```

### 11.4 导入策略

| 策略 | 说明 |
|------|------|
| `create-only` | 仅创建新模版，已存在的跳过 |
| `update-only` | 仅更新已存在的模版，新模版跳过 |
| `create-or-update` | 创建新模版或更新已存在的模版 (默认) |

### 11.5 CLI 支持 (可选)

```bash
# 通过 CLI 导入模版
liteboxd template import -f templates.yaml --prepull

# 导出所有模版
liteboxd template export > templates.yaml

# 导出单个模版
liteboxd template export python > python.yaml
```

## 12. 确定的技术决策

| 决策项 | 选择 | 说明 |
|--------|------|------|
| 数据存储 | **SQLite** | 轻量级，无需额外部署 |
| 多租户 | **暂不支持** | 保持简单，后续可扩展 |
| 镜像预拉取 | **支持** | 通过 DaemonSet 实现 |
| YAML 导入 | **支持** | 支持单个和批量导入 |
| Dockerfile 构建 | **不支持** | 直接使用现有镜像 |

## 13. 更新后的实现计划

### Phase 1: 核心模版功能 (优先)
- [x] 确定技术方案
- [ ] 新增 `internal/store/sqlite.go` - SQLite 数据库初始化
- [ ] 新增 `internal/model/template.go` - 模版数据模型
- [ ] 新增 `internal/store/template.go` - 模版数据访问层
- [ ] 新增 `internal/service/template.go` - 模版业务逻辑
- [ ] 新增 `internal/handler/template.go` - 模版 API 处理器
- [ ] 修改 `internal/service/sandbox.go` - 支持从模版创建
- [ ] 修改 `cmd/server/main.go` - 初始化模版服务

### Phase 2: 镜像预拉取
- [ ] 新增 `internal/service/prepull.go` - 预拉取服务
- [ ] 新增 `internal/handler/prepull.go` - 预拉取 API
- [ ] 实现 DaemonSet 创建和状态查询
- [ ] 模版创建时自动预拉取选项

### Phase 3: YAML 导入导出
- [ ] 实现 YAML 解析和验证
- [ ] 实现导入 API (支持三种策略)
- [ ] 实现导出 API
- [ ] 导入时自动预拉取镜像

### Phase 4: 高级功能
- [ ] 启动脚本执行
- [ ] 文件预置上传
- [ ] 就绪探针检查

### Phase 5: 前端界面
- [ ] 模版列表页
- [ ] 模版详情页
- [ ] 从模版创建沙箱
- [ ] YAML 导入界面
- [ ] 镜像预拉取状态

### Phase 6: 内置模版
- [ ] `base` - Alpine 基础环境
- [ ] `python` - Python 3.11
- [ ] `python-data-science` - 数据科学环境
- [ ] `nodejs` - Node.js 20
- [ ] `golang` - Go 1.21

---

**文档版本**: v1.1
**更新日期**: 2025-01-24
**作者**: Claude Code
**参考**: [E2B Sandbox Template](https://e2b.dev/docs/sandbox-template)
