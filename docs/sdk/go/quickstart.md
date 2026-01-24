# LiteBoxd Go SDK - Quick Start Guide

Get started with the LiteBoxd Go SDK in minutes.

---

## Installation

```bash
go get github.com/fslongjin/liteboxd/sdk/go
```

---

## Basic Setup

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    liteboxd "github.com/fslongjin/liteboxd/sdk/go"
    "github.com/fslongjin/liteboxd/backend/internal/model"
)

func main() {
    // Create a client
    client := liteboxd.NewClient(
        "http://localhost:8080/api/v1",
        liteboxd.WithTimeout(30*time.Second),
    )

    ctx := context.Background()
}
```

---

## Creating a Sandbox

**Important**: Sandbox creation is now template-only. You must create a template first, then create sandboxes from it.

### From Template (Latest Version)

```go
// Create with default template configuration
sandbox, err := client.Sandbox.Create(ctx, "python-data-science", nil)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Created sandbox: %s\n", sandbox.ID)
```

### From Template with Overrides

```go
// Override specific values
sandbox, err := client.Sandbox.Create(ctx, "python-data-science", &model.SandboxOverrides{
    CPU:    "1000m",
    Memory: "1Gi",
    TTL:    7200,
    Env: map[string]string{
        "DEBUG": "true",
    },
})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Created sandbox: %s\n", sandbox.ID)
```

**Note**: Only `CPU`, `Memory`, `TTL`, and `Env` can be overridden. The following are inherited from template and cannot be changed:
- `Image`
- `StartupScript`
- `Files`
- `ReadinessProbe`

### From Specific Template Version

```go
// Create from a specific template version
sandbox, err := client.Sandbox.CreateWithVersion(ctx, "python-data-science", 2, nil)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Created sandbox from version 2: %s\n", sandbox.ID)
```

### Wait for Ready

```go
sandbox, err := client.Sandbox.WaitForReady(ctx, sandbox.ID, 2*time.Second, 5*time.Minute)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Sandbox is ready: %s\n", sandbox.Status)
```

---

## Executing Commands

```go
resp, err := client.Sandbox.Execute(ctx, sandbox.ID, []string{"python", "-c", "print('hello')"}, 30)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Exit code: %d\n", resp.ExitCode)
fmt.Printf("Stdout: %s\n", resp.Stdout)
if resp.Stderr != "" {
    fmt.Printf("Stderr: %s\n", resp.Stderr)
}
```

---

## File Operations

### Upload File

```go
content := []byte("print('hello from file')")
err = client.Sandbox.UploadFile(ctx, sandbox.ID, "/workspace/app.py", content, "text/x-python")
```

### Download File

```go
downloaded, err := client.Sandbox.DownloadFile(ctx, sandbox.ID, "/workspace/output.txt")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Downloaded: %s\n", string(downloaded))
```

---

## Listing Sandboxes

```go
sandboxes, err := client.Sandbox.List(ctx)
if err != nil {
    log.Fatal(err)
}

for _, sb := range sandboxes {
    fmt.Printf("%s - %s - %s\n", sb.ID, sb.Image, sb.Status)
}
```

---

## Getting Sandbox Details

```go
sandbox, err := client.Sandbox.Get(ctx, sandboxID)
if err != nil {
    if errors.Is(err, liteboxd.ErrNotFound) {
        log.Fatal("Sandbox not found")
    }
    log.Fatal(err)
}

fmt.Printf("ID: %s\n", sandbox.ID)
fmt.Printf("Status: %s\n", sandbox.Status)
fmt.Printf("Expires: %s\n", sandbox.ExpiresAt)
```

---

## Deleting a Sandbox

```go
err = client.Sandbox.Delete(ctx, sandboxID)
if err != nil {
    log.Fatal(err)
}
```

---

## Template Management

### Create Template

```go
template, err := client.Template.Create(ctx, &model.CreateTemplateRequest{
    Name:        "python-basic",
    DisplayName: "Python Basic",
    Description: "Basic Python environment",
    Tags:        []string{"python", "basic"},
    Spec: model.TemplateSpec{
        Image: "python:3.11-slim",
        Resources: model.ResourceSpec{
            CPU:    "500m",
            Memory: "512Mi",
        },
        TTL:   3600,
        Env: map[string]string{
            "PYTHONUNBUFFERED": "1",
        },
    },
    AutoPrepull: true,
})
```

### List Templates

```go
list, err := client.Template.List(ctx, &model.TemplateListOptions{
    Tag:  "python",
    Page: 1,
})
if err != nil {
    log.Fatal(err)
}

for _, tpl := range list.Items {
    fmt.Printf("%s - %s\n", tpl.Name, tpl.DisplayName)
}
```

### Get Template

```go
template, err := client.Template.Get(ctx, "python-basic")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Name: %s\n", template.Name)
fmt.Printf("Latest Version: %d\n", template.LatestVersion)
```

### Update Template

```go
updated, err := client.Template.Update(ctx, "python-basic", &model.UpdateTemplateRequest{
    DisplayName: "Python Basic (Updated)",
    Spec: model.TemplateSpec{
        Image: "python:3.12-slim",
        Resources: model.ResourceSpec{
            CPU:    "500m",
            Memory: "512Mi",
        },
        TTL: 3600,
    },
    Changelog: "Upgrade to Python 3.12",
})
```

### Rollback Template

```go
rollback, err := client.Template.Rollback(ctx, "python-basic", 1, "Revert to 3.11")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Rolled back from version %d to %d\n", rollback.RolledBackFrom, rollback.RolledBackTo)
```

### Delete Template

```go
err = client.Template.Delete(ctx, "python-basic")
```

---

## Image Prepull

### Trigger Prepull

```go
task, err := client.Prepull.Create(ctx, "python:3.11-slim", 600)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Prepull task ID: %s\n", task.ID)
```

### Wait for Completion

```go
task, err := client.Prepull.WaitForCompletion(ctx, task.ID, 5*time.Second, 30*time.Minute)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Prepull completed: %s\n", task.Status)
```

### List Prepull Tasks

```go
tasks, err := client.Prepull.List(ctx, "", "")
for _, task := range tasks {
    fmt.Printf("%s - %s - %d/%d nodes\n", task.ID, task.Image, task.Progress.Ready, task.Progress.Total)
}
```

---

## Import/Export

### Export Template

```go
yaml, err := client.Template.ExportYAML(ctx, "python-basic", 0)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("%s\n", yaml)
```

### Import Templates

```go
yamlContent := []byte(`
apiVersion: liteboxd/v1
kind: SandboxTemplate
metadata:
  name: python-basic
spec:
  image: python:3.11-slim
  resources:
    cpu: "500m"
    memory: "512Mi"
  ttl: 3600
`)

result, err := client.ImportExport.ImportTemplates(ctx, yamlContent, "create-or-update", true)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Imported: %d created, %d updated\n", result.Created, result.Updated)
```

---

## Complete Example: Batch Job Runner

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    liteboxd "github.com/fslongjin/liteboxd/sdk/go"
    "github.com/fslongjin/liteboxd/backend/internal/model"
)

func main() {
    client := liteboxd.NewClient(
        "http://localhost:8080/api/v1",
        liteboxd.WithTimeout(30*time.Second),
    )
    ctx := context.Background()

    // Create sandbox from template
    sandbox, err := client.Sandbox.Create(ctx, "python-data-science", &model.SandboxOverrides{
        TTL: 3600,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Sandbox.Delete(ctx, sandbox.ID)

    // Wait for ready
    sandbox, err = client.Sandbox.WaitForReady(ctx, sandbox.ID, 2*time.Second, 5*time.Minute)
    if err != nil {
        log.Fatal(err)
    }

    // Upload script
    script := []byte(`
import numpy as np
print(np.array([1, 2, 3]))
`)
    err = client.Sandbox.UploadFile(ctx, sandbox.ID, "/workspace/script.py", script, "text/x-python")
    if err != nil {
        log.Fatal(err)
    }

    // Execute
    resp, err := client.Sandbox.Execute(ctx, sandbox.ID, []string{"python", "/workspace/script.py"}, 60)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Stdout)
}
```

---

## Next Steps

- See [design.md](design.md) for detailed architecture
- See [api-reference.md](api-reference.md) for complete API reference
- Check the `examples/` directory for more code samples
