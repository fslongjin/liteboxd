# 沙箱模版 API 规范

## 概述

本文档定义沙箱模版系统的完整 API 规范，遵循 RESTful 设计原则。

## 基础信息

- **Base URL**: `/api/v1`
- **Content-Type**: `application/json`
- **认证**: 预留 (当前无认证)

---

## 模版管理 API

### 1. 创建模版

创建新的沙箱模版。

```http
POST /templates
```

**请求体**:

```json
{
  "name": "python-data-science",
  "displayName": "Python 数据科学环境",
  "description": "预装 numpy, pandas, matplotlib 的 Python 环境",
  "tags": ["python", "data-science"],
  "isPublic": true,
  "spec": {
    "image": "python:3.11-slim",
    "resources": {
      "cpu": "500m",
      "memory": "512Mi"
    },
    "ttl": 3600,
    "env": {
      "PYTHONUNBUFFERED": "1",
      "PIP_NO_CACHE_DIR": "1"
    },
    "startupScript": "pip install numpy pandas matplotlib",
    "startupTimeout": 300,
    "files": [
      {
        "content": "print('Hello from template!')",
        "destination": "/workspace/hello.py"
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
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 模版唯一标识，只允许小写字母、数字、中划线 |
| displayName | string | 否 | 显示名称 |
| description | string | 否 | 模版描述 |
| tags | string[] | 否 | 标签列表 |
| isPublic | boolean | 否 | 是否公开，默认 true |
| spec | object | 是 | 模版规格配置 |
| spec.image | string | 是 | 容器镜像地址 |
| spec.resources | object | 否 | 资源配置 |
| spec.resources.cpu | string | 否 | CPU 限制，默认 "500m" |
| spec.resources.memory | string | 否 | 内存限制，默认 "512Mi" |
| spec.ttl | integer | 否 | 默认 TTL 秒数，默认 3600 |
| spec.env | object | 否 | 环境变量键值对 |
| spec.startupScript | string | 否 | 启动脚本 |
| spec.startupTimeout | integer | 否 | 启动脚本超时秒数，默认 300 |
| spec.files | array | 否 | 预置文件列表 |
| spec.readinessProbe | object | 否 | 就绪探针配置 |

**响应**: `201 Created`

```json
{
  "id": "tpl-a1b2c3d4",
  "name": "python-data-science",
  "displayName": "Python 数据科学环境",
  "description": "预装 numpy, pandas, matplotlib 的 Python 环境",
  "tags": ["python", "data-science"],
  "author": "",
  "isPublic": true,
  "latestVersion": 1,
  "createdAt": "2025-01-24T10:00:00Z",
  "updatedAt": "2025-01-24T10:00:00Z"
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 请求参数无效 |
| 409 | 模版名称已存在 |

---

### 2. 列出模版

获取所有模版列表。

```http
GET /templates
```

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| tag | string | 按标签筛选 |
| search | string | 搜索名称或描述 |
| page | integer | 页码，默认 1 |
| pageSize | integer | 每页数量，默认 20，最大 100 |

**响应**: `200 OK`

```json
{
  "items": [
    {
      "id": "tpl-a1b2c3d4",
      "name": "python-data-science",
      "displayName": "Python 数据科学环境",
      "description": "预装 numpy, pandas, matplotlib 的 Python 环境",
      "tags": ["python", "data-science"],
      "isPublic": true,
      "latestVersion": 2,
      "createdAt": "2025-01-24T10:00:00Z",
      "updatedAt": "2025-01-24T12:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "pageSize": 20
}
```

---

### 3. 获取模版详情

获取指定模版的详细信息，包括最新版本的完整规格。

```http
GET /templates/{name}
```

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 模版名称 |

**响应**: `200 OK`

```json
{
  "id": "tpl-a1b2c3d4",
  "name": "python-data-science",
  "displayName": "Python 数据科学环境",
  "description": "预装 numpy, pandas, matplotlib 的 Python 环境",
  "tags": ["python", "data-science"],
  "author": "",
  "isPublic": true,
  "latestVersion": 2,
  "createdAt": "2025-01-24T10:00:00Z",
  "updatedAt": "2025-01-24T12:00:00Z",
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
    "startupScript": "pip install numpy pandas matplotlib scikit-learn",
    "startupTimeout": 300
  }
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 404 | 模版不存在 |

---

### 4. 更新模版

更新模版配置，会自动创建新版本。

```http
PUT /templates/{name}
```

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 模版名称 |

**请求体**:

```json
{
  "displayName": "Python 数据科学环境 (升级版)",
  "description": "预装更多数据科学工具包",
  "tags": ["python", "data-science", "ml"],
  "spec": {
    "image": "python:3.11-slim",
    "resources": {
      "cpu": "1000m",
      "memory": "1Gi"
    },
    "ttl": 7200,
    "env": {
      "PYTHONUNBUFFERED": "1"
    },
    "startupScript": "pip install numpy pandas matplotlib scikit-learn tensorflow"
  },
  "changelog": "添加 tensorflow 支持，增加资源配额"
}
```

**响应**: `200 OK`

```json
{
  "id": "tpl-a1b2c3d4",
  "name": "python-data-science",
  "displayName": "Python 数据科学环境 (升级版)",
  "latestVersion": 3,
  "updatedAt": "2025-01-24T14:00:00Z"
}
```

---

### 5. 删除模版

删除模版及其所有版本。

```http
DELETE /templates/{name}
```

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 模版名称 |

**响应**: `204 No Content`

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 404 | 模版不存在 |

---

## 版本管理 API

### 6. 列出模版版本

获取模版的所有历史版本。

```http
GET /templates/{name}/versions
```

**响应**: `200 OK`

```json
{
  "items": [
    {
      "id": "ver-x1y2z3",
      "templateId": "tpl-a1b2c3d4",
      "version": 3,
      "changelog": "添加 tensorflow 支持",
      "createdAt": "2025-01-24T14:00:00Z",
      "createdBy": ""
    },
    {
      "id": "ver-a1b2c3",
      "templateId": "tpl-a1b2c3d4",
      "version": 2,
      "changelog": "增加 scikit-learn",
      "createdAt": "2025-01-24T12:00:00Z",
      "createdBy": ""
    },
    {
      "id": "ver-m1n2o3",
      "templateId": "tpl-a1b2c3d4",
      "version": 1,
      "changelog": "初始版本",
      "createdAt": "2025-01-24T10:00:00Z",
      "createdBy": ""
    }
  ],
  "total": 3
}
```

---

### 7. 获取特定版本

获取模版的特定版本详情。

```http
GET /templates/{name}/versions/{version}
```

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 模版名称 |
| version | integer | 版本号 |

**响应**: `200 OK`

```json
{
  "id": "ver-a1b2c3",
  "templateId": "tpl-a1b2c3d4",
  "version": 2,
  "changelog": "增加 scikit-learn",
  "createdAt": "2025-01-24T12:00:00Z",
  "createdBy": "",
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
    "startupScript": "pip install numpy pandas matplotlib scikit-learn"
  }
}
```

---

### 8. 回滚版本

将模版回滚到指定的历史版本，会创建一个新版本。

```http
POST /templates/{name}/rollback
```

**请求体**:

```json
{
  "targetVersion": 1,
  "changelog": "回滚到 v1，移除不稳定的依赖"
}
```

**响应**: `200 OK`

```json
{
  "id": "tpl-a1b2c3d4",
  "name": "python-data-science",
  "latestVersion": 4,
  "rolledBackFrom": 3,
  "rolledBackTo": 1,
  "updatedAt": "2025-01-24T16:00:00Z"
}
```

---

## 扩展的沙箱 API

### 9. 从模版创建沙箱

扩展现有的创建沙箱 API，支持从模版创建。

```http
POST /sandboxes
```

**方式一: 直接配置 (现有方式)**

```json
{
  "image": "python:3.11-slim",
  "cpu": "500m",
  "memory": "512Mi",
  "ttl": 3600,
  "env": {
    "DEBUG": "true"
  }
}
```

**方式二: 从模版创建 (新增)**

```json
{
  "template": "python-data-science",
  "templateVersion": 2,
  "overrides": {
    "cpu": "1000m",
    "memory": "1Gi",
    "ttl": 7200,
    "env": {
      "DEBUG": "true",
      "EXTRA_VAR": "value"
    }
  }
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| template | string | 是* | 模版名称 (*与 image 二选一) |
| templateVersion | integer | 否 | 模版版本，默认使用最新版本 |
| overrides | object | 否 | 覆盖配置 |
| overrides.cpu | string | 否 | 覆盖 CPU 限制 |
| overrides.memory | string | 否 | 覆盖内存限制 |
| overrides.ttl | integer | 否 | 覆盖 TTL |
| overrides.env | object | 否 | 合并/覆盖环境变量 |

**响应**: `201 Created`

```json
{
  "id": "sbx-12345678",
  "image": "python:3.11-slim",
  "cpu": "1000m",
  "memory": "1Gi",
  "ttl": 7200,
  "status": "pending",
  "template": "python-data-science",
  "templateVersion": 2,
  "createdAt": "2025-01-24T10:00:00Z",
  "expiresAt": "2025-01-24T12:00:00Z"
}
```

**配置合并规则**:

1. 基础配置取自模版 spec
2. `overrides` 中的字段覆盖模版配置
3. `env` 采用合并策略:
   - 同名变量: overrides 覆盖模版
   - 不同名变量: 两边都保留

---

## 镜像预拉取 API

### 10. 触发镜像预拉取

在所有 K8s 节点上预拉取指定镜像。

```http
POST /images/prepull
```

**请求体**:

```json
{
  "image": "python:3.11-slim",
  "timeout": 600
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| image | string | 是 | 镜像地址 |
| timeout | integer | 否 | 超时秒数，默认 600 |

**响应**: `202 Accepted`

```json
{
  "id": "pp-a1b2c3d4",
  "image": "python:3.11-slim",
  "status": "pending",
  "startedAt": "2025-01-24T10:00:00Z"
}
```

---

### 11. 查询预拉取状态

获取所有镜像预拉取任务的状态。

```http
GET /images/prepull
```

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| image | string | 按镜像名称筛选 |
| status | string | 按状态筛选 (pending/pulling/completed/failed) |

**响应**: `200 OK`

```json
{
  "items": [
    {
      "id": "pp-a1b2c3d4",
      "image": "python:3.11-slim",
      "status": "completed",
      "progress": {
        "ready": 3,
        "total": 3
      },
      "startedAt": "2025-01-24T10:00:00Z",
      "completedAt": "2025-01-24T10:02:30Z"
    },
    {
      "id": "pp-x1y2z3",
      "image": "node:20-slim",
      "status": "pulling",
      "progress": {
        "ready": 1,
        "total": 3
      },
      "startedAt": "2025-01-24T10:05:00Z"
    }
  ]
}
```

**状态说明**:

| 状态 | 说明 |
|------|------|
| pending | 任务已创建，等待执行 |
| pulling | 正在拉取镜像 |
| completed | 所有节点拉取完成 |
| failed | 拉取失败 |

---

### 12. 取消/删除预拉取任务

删除指定的预拉取任务及其关联的 DaemonSet。

```http
DELETE /images/prepull/{id}
```

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 预拉取任务 ID |

**响应**: `204 No Content`

---

### 13. 预拉取模版镜像

预拉取指定模版使用的镜像。

```http
POST /templates/{name}/prepull
```

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 模版名称 |

**响应**: `202 Accepted`

```json
{
  "id": "pp-m1n2o3",
  "image": "python:3.11-slim",
  "template": "python-data-science",
  "status": "pending",
  "startedAt": "2025-01-24T10:00:00Z"
}
```

---

## YAML 导入导出 API

### 14. 导入模版

通过 YAML 文件导入一个或多个模版。

```http
POST /templates/import
Content-Type: multipart/form-data
```

**表单字段**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| file | file | 是 | YAML 文件 |
| strategy | string | 否 | 导入策略，默认 "create-or-update" |
| prepull | boolean | 否 | 是否自动预拉取镜像，默认 false |

**导入策略**:

| 策略 | 说明 |
|------|------|
| create-only | 仅创建新模版，已存在的跳过 |
| update-only | 仅更新已存在的模版，新模版跳过 |
| create-or-update | 创建新模版或更新已存在的模版 (默认) |

**YAML 文件格式 - 单个模版**:

```yaml
apiVersion: liteboxd/v1
kind: SandboxTemplate
metadata:
  name: python-data-science
  displayName: Python 数据科学环境
  description: 预装数据科学工具包
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

**YAML 文件格式 - 批量模版**:

```yaml
apiVersion: liteboxd/v1
kind: SandboxTemplateList
items:
  - metadata:
      name: python
      displayName: Python 3.11
    spec:
      image: python:3.11-slim
  - metadata:
      name: nodejs
      displayName: Node.js 20
    spec:
      image: node:20-slim
```

**响应**: `200 OK`

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
  "prepullStarted": ["python:3.11-slim", "node:20-slim"]
}
```

**错误响应** (部分失败):

```json
{
  "total": 3,
  "created": 1,
  "updated": 0,
  "skipped": 0,
  "failed": 2,
  "results": [
    {"name": "python", "action": "created", "version": 1},
    {"name": "invalid-name!", "action": "failed", "error": "invalid template name"},
    {"name": "nodejs", "action": "failed", "error": "invalid image format"}
  ]
}
```

---

### 15. 导出所有模版

将所有模版导出为 YAML 格式。

```http
GET /templates/export
```

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| tag | string | 按标签筛选 |
| names | string | 逗号分隔的模版名称列表 |

**响应**: `200 OK`

```yaml
Content-Type: application/x-yaml

apiVersion: liteboxd/v1
kind: SandboxTemplateList
exportedAt: "2025-01-24T10:00:00Z"
items:
  - metadata:
      name: python
      displayName: Python 3.11
      description: Python 3.11 runtime environment
      tags:
        - python
    spec:
      image: python:3.11-slim
      resources:
        cpu: "500m"
        memory: "512Mi"
      ttl: 3600
  - metadata:
      name: nodejs
      displayName: Node.js 20
    spec:
      image: node:20-slim
```

---

### 16. 导出单个模版

将指定模版导出为 YAML 格式。

```http
GET /templates/{name}/export
```

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 模版名称 |

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| version | integer | 导出指定版本，默认最新版本 |

**响应**: `200 OK`

```yaml
Content-Type: application/x-yaml

apiVersion: liteboxd/v1
kind: SandboxTemplate
exportedAt: "2025-01-24T10:00:00Z"
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

---

## 错误响应格式

所有错误响应遵循统一格式:

```json
{
  "error": {
    "code": "TEMPLATE_NOT_FOUND",
    "message": "Template 'python-data-science' not found",
    "details": {}
  }
}
```

**错误代码**:

| 代码 | HTTP 状态码 | 说明 |
|------|-------------|------|
| INVALID_REQUEST | 400 | 请求参数无效 |
| INVALID_YAML | 400 | YAML 格式无效 |
| TEMPLATE_NOT_FOUND | 404 | 模版不存在 |
| VERSION_NOT_FOUND | 404 | 版本不存在 |
| PREPULL_NOT_FOUND | 404 | 预拉取任务不存在 |
| TEMPLATE_EXISTS | 409 | 模版名称已存在 |
| PREPULL_IN_PROGRESS | 409 | 该镜像已有预拉取任务进行中 |
| INTERNAL_ERROR | 500 | 内部服务错误 |

---

## OpenAPI 规范片段

以下是需要添加到现有 `api/openapi.yaml` 的内容:

```yaml
paths:
  /templates:
    get:
      summary: List templates
      tags: [Templates]
      parameters:
        - name: tag
          in: query
          schema:
            type: string
        - name: search
          in: query
          schema:
            type: string
        - name: page
          in: query
          schema:
            type: integer
            default: 1
        - name: pageSize
          in: query
          schema:
            type: integer
            default: 20
      responses:
        '200':
          description: Template list
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TemplateListResponse'
    post:
      summary: Create template
      tags: [Templates]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateTemplateRequest'
      responses:
        '201':
          description: Template created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Template'

  /templates/{name}:
    parameters:
      - name: name
        in: path
        required: true
        schema:
          type: string
    get:
      summary: Get template
      tags: [Templates]
      responses:
        '200':
          description: Template details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TemplateDetail'
        '404':
          description: Template not found
    put:
      summary: Update template
      tags: [Templates]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/UpdateTemplateRequest'
      responses:
        '200':
          description: Template updated
    delete:
      summary: Delete template
      tags: [Templates]
      responses:
        '204':
          description: Template deleted

components:
  schemas:
    Template:
      type: object
      properties:
        id:
          type: string
        name:
          type: string
        displayName:
          type: string
        description:
          type: string
        tags:
          type: array
          items:
            type: string
        isPublic:
          type: boolean
        latestVersion:
          type: integer
        createdAt:
          type: string
          format: date-time
        updatedAt:
          type: string
          format: date-time

    TemplateSpec:
      type: object
      required:
        - image
      properties:
        image:
          type: string
        resources:
          type: object
          properties:
            cpu:
              type: string
              default: "500m"
            memory:
              type: string
              default: "512Mi"
        ttl:
          type: integer
          default: 3600
        env:
          type: object
          additionalProperties:
            type: string
        startupScript:
          type: string
        startupTimeout:
          type: integer
          default: 300
        files:
          type: array
          items:
            $ref: '#/components/schemas/FileSpec'
        readinessProbe:
          $ref: '#/components/schemas/ProbeSpec'

    FileSpec:
      type: object
      properties:
        source:
          type: string
        destination:
          type: string
        content:
          type: string

    ProbeSpec:
      type: object
      properties:
        exec:
          type: object
          properties:
            command:
              type: array
              items:
                type: string
        initialDelaySeconds:
          type: integer
        periodSeconds:
          type: integer
        failureThreshold:
          type: integer

    CreateTemplateRequest:
      type: object
      required:
        - name
        - spec
      properties:
        name:
          type: string
          pattern: '^[a-z0-9][a-z0-9-]*[a-z0-9]$'
        displayName:
          type: string
        description:
          type: string
        tags:
          type: array
          items:
            type: string
        isPublic:
          type: boolean
          default: true
        spec:
          $ref: '#/components/schemas/TemplateSpec'
```

---

**文档版本**: v1.1
**更新日期**: 2025-01-24
