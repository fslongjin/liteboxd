# 数据库设计文档

## 概述

本文档描述沙箱模版系统的数据库设计，采用 SQLite 作为默认存储方案，支持扩展到 PostgreSQL。

## 数据库选型

### SQLite (推荐用于轻量级部署)

**优势**:
- 零配置，无需额外部署数据库服务
- 单文件存储，便于备份和迁移
- 对于中小规模模版管理足够使用
- Go 标准库 `database/sql` 原生支持

**限制**:
- 不适合高并发写入场景
- 不支持跨机器部署

### PostgreSQL (可选，用于生产环境)

**优势**:
- 高并发支持
- 完善的 JSON 查询能力
- 支持分布式部署

## 表结构设计

### templates 表 - 模版主表

```sql
CREATE TABLE IF NOT EXISTS templates (
    -- 主键
    id TEXT PRIMARY KEY,

    -- 基本信息
    name TEXT NOT NULL UNIQUE,           -- 模版唯一标识
    display_name TEXT,                   -- 显示名称
    description TEXT,                    -- 描述
    tags TEXT DEFAULT '[]',              -- 标签 (JSON 数组)
    author TEXT DEFAULT '',              -- 作者
    is_public BOOLEAN DEFAULT TRUE,      -- 是否公开

    -- 版本信息
    latest_version INTEGER DEFAULT 0,    -- 最新版本号

    -- 时间戳
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_templates_name ON templates(name);
CREATE INDEX IF NOT EXISTS idx_templates_is_public ON templates(is_public);
CREATE INDEX IF NOT EXISTS idx_templates_created_at ON templates(created_at);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | UUID 格式的主键，如 "tpl-a1b2c3d4" |
| name | TEXT | 模版唯一标识，如 "python-data-science" |
| display_name | TEXT | 友好显示名称 |
| description | TEXT | 模版描述 |
| tags | TEXT | JSON 数组格式的标签列表 |
| author | TEXT | 创建者 |
| is_public | BOOLEAN | 是否公开可用 |
| latest_version | INTEGER | 当前最新版本号 |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 最后更新时间 |

### template_versions 表 - 模版版本表

```sql
CREATE TABLE IF NOT EXISTS template_versions (
    -- 主键
    id TEXT PRIMARY KEY,

    -- 外键关联
    template_id TEXT NOT NULL,

    -- 版本信息
    version INTEGER NOT NULL,            -- 版本号 (1, 2, 3...)

    -- 规格配置 (JSON 格式)
    spec TEXT NOT NULL,                  -- TemplateSpec JSON

    -- 变更信息
    changelog TEXT DEFAULT '',           -- 版本变更说明
    created_by TEXT DEFAULT '',          -- 创建者

    -- 时间戳
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- 外键约束
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,

    -- 唯一约束: 同一模版的版本号不能重复
    UNIQUE(template_id, version)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_template_versions_template_id
    ON template_versions(template_id);
CREATE INDEX IF NOT EXISTS idx_template_versions_version
    ON template_versions(template_id, version DESC);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | UUID 格式的主键，如 "ver-x1y2z3" |
| template_id | TEXT | 关联的模版 ID |
| version | INTEGER | 版本号，从 1 开始递增 |
| spec | TEXT | JSON 格式的 TemplateSpec |
| changelog | TEXT | 版本变更说明 |
| created_by | TEXT | 创建此版本的用户 |
| created_at | TIMESTAMP | 创建时间 |

### TemplateSpec JSON 结构

```json
{
  "image": "python:3.11-slim",
  "resources": {
    "cpu": "500m",
    "memory": "512Mi"
  },
  "ttl": 3600,
  "env": {
    "PYTHONUNBUFFERED": "1"
  },
  "startupScript": "pip install numpy pandas",
  "startupTimeout": 300,
  "files": [
    {
      "source": "",
      "destination": "/workspace/hello.py",
      "content": "print('Hello!')"
    }
  ],
  "readinessProbe": {
    "exec": {
      "command": ["python", "-c", "import numpy"]
    },
    "initialDelaySeconds": 5,
    "periodSeconds": 2,
    "failureThreshold": 30
  }
}
```

## 数据迁移

### 初始化脚本

```sql
-- migrations/001_init_templates.sql

-- 创建 templates 表
CREATE TABLE IF NOT EXISTS templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT,
    description TEXT,
    tags TEXT DEFAULT '[]',
    author TEXT DEFAULT '',
    is_public BOOLEAN DEFAULT TRUE,
    latest_version INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建 template_versions 表
CREATE TABLE IF NOT EXISTS template_versions (
    id TEXT PRIMARY KEY,
    template_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    spec TEXT NOT NULL,
    changelog TEXT DEFAULT '',
    created_by TEXT DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,
    UNIQUE(template_id, version)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_templates_name ON templates(name);
CREATE INDEX IF NOT EXISTS idx_templates_is_public ON templates(is_public);
CREATE INDEX IF NOT EXISTS idx_template_versions_template_id ON template_versions(template_id);
```

## Go 数据模型

### internal/model/template.go

```go
package model

import (
	"encoding/json"
	"time"
)

// Template 模版主表模型
type Template struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DisplayName   string    `json:"displayName"`
	Description   string    `json:"description"`
	Tags          []string  `json:"tags"`
	Author        string    `json:"author"`
	IsPublic      bool      `json:"isPublic"`
	LatestVersion int       `json:"latestVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`

	// 非持久化字段，用于 API 响应
	Spec          *TemplateSpec `json:"spec,omitempty"`
}

// TemplateVersion 模版版本模型
type TemplateVersion struct {
	ID         string       `json:"id"`
	TemplateID string       `json:"templateId"`
	Version    int          `json:"version"`
	Spec       TemplateSpec `json:"spec"`
	Changelog  string       `json:"changelog"`
	CreatedBy  string       `json:"createdBy"`
	CreatedAt  time.Time    `json:"createdAt"`
}

// TemplateSpec 模版规格
type TemplateSpec struct {
	Image          string            `json:"image"`
	Resources      ResourceSpec      `json:"resources"`
	TTL            int               `json:"ttl"`
	Env            map[string]string `json:"env,omitempty"`
	StartupScript  string            `json:"startupScript,omitempty"`
	StartupTimeout int               `json:"startupTimeout,omitempty"`
	Files          []FileSpec        `json:"files,omitempty"`
	ReadinessProbe *ProbeSpec        `json:"readinessProbe,omitempty"`
}

// ResourceSpec 资源规格
type ResourceSpec struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// FileSpec 文件规格
type FileSpec struct {
	Source      string `json:"source,omitempty"`      // 文件路径 (与 Content 二选一)
	Destination string `json:"destination"`           // 目标路径
	Content     string `json:"content,omitempty"`     // 内联内容 (与 Source 二选一)
}

// ProbeSpec 探针规格
type ProbeSpec struct {
	Exec                ExecAction `json:"exec"`
	InitialDelaySeconds int        `json:"initialDelaySeconds"`
	PeriodSeconds       int        `json:"periodSeconds"`
	FailureThreshold    int        `json:"failureThreshold"`
}

// ExecAction 执行动作
type ExecAction struct {
	Command []string `json:"command"`
}

// MarshalTags 将 Tags 序列化为 JSON 字符串 (用于数据库存储)
func (t *Template) MarshalTags() string {
	if t.Tags == nil {
		return "[]"
	}
	data, _ := json.Marshal(t.Tags)
	return string(data)
}

// UnmarshalTags 从 JSON 字符串反序列化 Tags
func (t *Template) UnmarshalTags(data string) error {
	return json.Unmarshal([]byte(data), &t.Tags)
}

// MarshalSpec 将 Spec 序列化为 JSON 字符串
func (v *TemplateVersion) MarshalSpec() string {
	data, _ := json.Marshal(v.Spec)
	return string(data)
}

// UnmarshalSpec 从 JSON 字符串反序列化 Spec
func (v *TemplateVersion) UnmarshalSpec(data string) error {
	return json.Unmarshal([]byte(data), &v.Spec)
}
```

### internal/model/template_request.go

```go
package model

// CreateTemplateRequest 创建模版请求
type CreateTemplateRequest struct {
	Name        string       `json:"name" binding:"required"`
	DisplayName string       `json:"displayName"`
	Description string       `json:"description"`
	Tags        []string     `json:"tags"`
	IsPublic    *bool        `json:"isPublic"`
	Spec        TemplateSpec `json:"spec" binding:"required"`
}

// UpdateTemplateRequest 更新模版请求
type UpdateTemplateRequest struct {
	DisplayName string       `json:"displayName"`
	Description string       `json:"description"`
	Tags        []string     `json:"tags"`
	IsPublic    *bool        `json:"isPublic"`
	Spec        TemplateSpec `json:"spec" binding:"required"`
	Changelog   string       `json:"changelog"`
}

// RollbackRequest 回滚请求
type RollbackRequest struct {
	TargetVersion int    `json:"targetVersion" binding:"required"`
	Changelog     string `json:"changelog"`
}

// TemplateListResponse 模版列表响应
type TemplateListResponse struct {
	Items    []Template `json:"items"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
}

// VersionListResponse 版本列表响应
type VersionListResponse struct {
	Items []TemplateVersion `json:"items"`
	Total int               `json:"total"`
}

// CreateSandboxFromTemplateRequest 从模版创建沙箱请求
type CreateSandboxFromTemplateRequest struct {
	Template        string           `json:"template"`
	TemplateVersion int              `json:"templateVersion,omitempty"`
	Overrides       *TemplateOverrides `json:"overrides,omitempty"`
}

// TemplateOverrides 模版覆盖配置
type TemplateOverrides struct {
	CPU    string            `json:"cpu,omitempty"`
	Memory string            `json:"memory,omitempty"`
	TTL    int               `json:"ttl,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}
```

## 数据访问层

### internal/store/template.go (接口定义)

```go
package store

import (
	"context"

	"github.com/yourorg/liteboxd/internal/model"
)

// TemplateStore 模版存储接口
type TemplateStore interface {
	// 模版 CRUD
	Create(ctx context.Context, template *model.Template, spec *model.TemplateSpec) error
	Get(ctx context.Context, name string) (*model.Template, error)
	GetByID(ctx context.Context, id string) (*model.Template, error)
	List(ctx context.Context, opts ListOptions) (*model.TemplateListResponse, error)
	Update(ctx context.Context, name string, req *model.UpdateTemplateRequest) (*model.Template, error)
	Delete(ctx context.Context, name string) error

	// 版本管理
	GetVersion(ctx context.Context, name string, version int) (*model.TemplateVersion, error)
	GetLatestVersion(ctx context.Context, name string) (*model.TemplateVersion, error)
	ListVersions(ctx context.Context, name string) (*model.VersionListResponse, error)
	CreateVersion(ctx context.Context, templateID string, version int, spec *model.TemplateSpec, changelog string) (*model.TemplateVersion, error)
}

// ListOptions 列表查询选项
type ListOptions struct {
	Tag      string
	Search   string
	Page     int
	PageSize int
}
```

## 查询示例

### 获取模版及其最新规格

```sql
SELECT
    t.id, t.name, t.display_name, t.description, t.tags,
    t.author, t.is_public, t.latest_version, t.created_at, t.updated_at,
    v.spec
FROM templates t
LEFT JOIN template_versions v
    ON t.id = v.template_id AND t.latest_version = v.version
WHERE t.name = ?
```

### 按标签搜索模版

```sql
SELECT id, name, display_name, description, tags, latest_version
FROM templates
WHERE is_public = TRUE
  AND tags LIKE '%"python"%'
ORDER BY created_at DESC
LIMIT ? OFFSET ?
```

### 获取模版的所有版本

```sql
SELECT id, template_id, version, spec, changelog, created_by, created_at
FROM template_versions
WHERE template_id = (SELECT id FROM templates WHERE name = ?)
ORDER BY version DESC
```

## 备份与恢复

### SQLite 备份

```bash
# 备份
cp data/liteboxd.db data/liteboxd.db.backup

# 或使用 SQLite 在线备份
sqlite3 data/liteboxd.db ".backup 'data/liteboxd.db.backup'"
```

### 导出为 SQL

```bash
sqlite3 data/liteboxd.db .dump > backup.sql
```

### 恢复

```bash
sqlite3 data/liteboxd.db < backup.sql
```

---

**文档版本**: v1.0
**创建日期**: 2025-01-24
