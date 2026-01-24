# LiteBoxd CLI - Command Reference

Complete command reference for the LiteBoxd CLI.

---

## Table of Contents

1. [Global Options](#1-global-options)
2. [Sandbox Commands](#2-sandbox-commands)
3. [Template Commands](#3-template-commands)
4. [Image Commands](#4-image-commands)
5. [Import Command](#5-import-command)
6. [Completion Command](#6-completion-command)
7. [Exit Codes](#7-exit-codes)

---

## 1. Global Options

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--api-server` | string | `http://localhost:8080/api/v1` | LiteBoxd API server address |
| `--output` / `-o` | string | `table` | Output format: `table`, `json`, `yaml` |
| `--timeout` | duration | `30s` | Request timeout |
| `--verbose` / `-v` | bool | `false` | Enable verbose output |
| `--config` | string | `~/.config/liteboxd/config.yaml` | Config file path |
| `--profile` | string | `default` | Configuration profile |
| `--help` / `-h` | bool | - | Show help |
| `--version` | bool | - | Show version |

---

## 2. Sandbox Commands

### `sandbox create`

Create a new sandbox from a template.

**Important**: Sandbox creation is template-only. You must specify a template using `--template`.

```bash
liteboxd sandbox create --template <name> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--template` / `-t` | string | Template name (required) |
| `--template-version` | int | Template version (default: latest) |
| `--cpu` | string | Override CPU limit (from template: 500m) |
| `--memory` | string | Override Memory limit (from template: 512Mi) |
| `--ttl` | int | Override time to live in seconds (from template: 3600) |
| `--env` | stringArray | Override/merge environment variables (KEY=VALUE) |
| `--wait` | bool | Wait for sandbox to be ready |
| `--timeout` | duration | Wait timeout (default: 5m) |
| `--quiet` / `-q` | bool | Only print sandbox ID |

**Notes**:
- Only `--cpu`, `--memory`, `--ttl`, and `--env` can override template values
- Image, startup script, files, and readiness probe come from template only

**Examples**:
```bash
# Create from template with defaults
liteboxd sandbox create --template python-data-science

# Create with overrides
liteboxd sandbox create --template python-data-science --ttl 7200 --env DEBUG=true

# Create from specific version
liteboxd sandbox create --template python-ds --template-version 2

# Create and wait for ready
liteboxd sandbox create --template nodejs --wait
```

### `sandbox list`

List all sandboxes.

```bash
liteboxd sandbox list [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--status` | string | Filter by status |
| `--output` / `-o` | string | Output format |

**Examples**:
```bash
liteboxd sandbox list
liteboxd sandbox list --status running --output json
```

### `sandbox get`

Get sandbox details.

```bash
liteboxd sandbox get <id> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--output` / `-o` | string | Output format |

### `sandbox delete`

Delete a sandbox.

```bash
liteboxd sandbox delete <id> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--force` / `-f` | bool | Skip confirmation |
| `--wait` | bool | Wait for deletion to complete |

### `sandbox exec`

Execute a command in a sandbox.

```bash
liteboxd sandbox exec <id> -- <command> [args...]
```

| Flag | Type | Description |
|------|------|-------------|
| `--timeout` | duration | Execution timeout (default: 30s) |
| `--quiet` | bool | Only print stdout |
| `--exit-code` | bool | Print exit code |

**Examples**:
```bash
# Python script
liteboxd sandbox exec <id> -- python -c "print('hello')"

# With arguments
liteboxd sandbox exec <id> -- npm install

# Long-running command
liteboxd sandbox exec <id> --timeout 5m -- npm test
```

### `sandbox logs`

Get sandbox logs.

```bash
liteboxd sandbox logs <id> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--events` | bool | Show Pod events |
| `--tail` | int | Number of lines (default: 100) |

### `sandbox upload`

Upload a file to a sandbox.

```bash
liteboxd sandbox upload <id> <local-path> <remote-path>
```

**Examples**:
```bash
liteboxd sandbox upload <id> ./main.py /workspace/main.py
```

### `sandbox download`

Download a file from a sandbox.

```bash
liteboxd sandbox download <id> <remote-path> [local-path]
```

**Examples**:
```bash
liteboxd sandbox download <id> /workspace/output.txt ./output.txt
```

### `sandbox wait`

Wait for a sandbox to be ready.

```bash
liteboxd sandbox wait <id> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--poll-interval` | duration | Poll interval (default: 2s) |
| `--timeout` | duration | Max wait time (default: 5m) |
| `--quiet` | bool | Only print status |

---

## 3. Template Commands

### `template create`

Create a new template.

```bash
liteboxd template create --name <name> --file <yaml-file> [flags]
```

Or use flags instead of file:

```bash
liteboxd template create [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--name` / `-n` | string | Template name (required) |
| `--file` / `-f` | string | YAML file with template spec |
| `--display-name` | string | Display name |
| `--description` / `-d` | string | Description |
| `--tags` | stringArray | Tags |
| `--image` | string | Container image (required if no --file) |
| `--cpu` | string | CPU limit (default: 500m) |
| `--memory` | string | Memory limit (default: 512Mi) |
| `--ttl` | int | Default TTL (default: 3600) |
| `--startup-script` | string | Startup script file |
| `--readiness-command` | string | Readiness probe command |
| `--public` / `--private` | bool | Set visibility |
| `--prepull` | bool | Auto-prepull image after creation |

**Examples**:
```bash
# Create from YAML file
liteboxd template create --name python-ds --file template.yaml

# Create using flags
liteboxd template create \
  --name python-ds \
  --image python:3.11-slim \
  --cpu 1000m --memory 1Gi \
  --tags python,data-science \
  --display-name "Python Data Science" \
  --description "Python with data science libraries"
```

### `template list`

List templates.

```bash
liteboxd template list [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--tag` | string | Filter by tag |
| `--search` | string | Search in name/description |
| `--page` | int | Page number (default: 1) |
| `--page-size` | int | Items per page (default: 20) |
| `--output` / `-o` | string | Output format |

### `template get`

Get template details.

```bash
liteboxd template get <name> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--version` / `-v` | int | Get specific version |
| `--output` / `-o` | string | Output format |

### `template update`

Update a template (creates new version).

```bash
liteboxd template update <name> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--file` / `-f` | string | YAML file with new spec |
| `--changelog` | string | Changelog for the update |
| `--display-name` | string | New display name |
| `--description` / `-d` | string | New description |
| `--tags` | stringArray | New tags |
| `--image` | string | New image |
| `--cpu` | string | New CPU limit |
| `--memory` | string | New memory limit |
| `--ttl` | int | New TTL |
| `--startup-script` | string | New startup script file |

### `template delete`

Delete a template.

```bash
liteboxd template delete <name> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--force` / `-f` | bool | Skip confirmation |

### `template versions`

List template versions.

```bash
liteboxd template versions <name> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--output` / `-o` | string | Output format |

### `template rollback`

Rollback template to a previous version.

```bash
liteboxd template rollback <name> --to <version> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--to` | int | Target version (required) |
| `--changelog` | string | Changelog for the rollback |

### `template export`

Export template(s) to YAML.

```bash
liteboxd template export [name] [flags]
```

If name is provided, exports single template; otherwise exports all.

| Flag | Type | Description |
|------|------|-------------|
| `--output` / `-o` | string | Output file (default: stdout) |
| `--version` / `-v` | int | Specific version for single export |
| `--tag` | string | Filter by tag (for all export) |
| `--names` | string | Comma-separated names (for all export) |

---

## 4. Image Commands

### `image prepull`

Trigger image prepull.

```bash
liteboxd image prepull <image> [flags]
```

Or from template:

```bash
liteboxd image prepull --template <name> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--template` / `-t` | string | Prepull image from template |
| `--timeout` | duration | Prepull timeout (default: 10m) |
| `--wait` | bool | Wait for completion |

### `image list`

List prepull tasks.

```bash
liteboxd image list [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--image` | string | Filter by image |
| `--status` | string | Filter by status |
| `--output` / `-o` | string | Output format |

### `image delete`

Delete a prepull task.

```bash
liteboxd image delete <id> [flags]
```

---

## 5. Import Command

### `import`

Import templates from YAML.

```bash
liteboxd import --file <yaml-file> [flags]
```

| Flag | Type | Description |
|------|------|-------------|
| `--file` / `-f` | string | YAML file (required) |
| `--strategy` | string | `create-only`, `update-only`, `create-or-update` (default) |
| `--prepull` | bool | Auto-prepull images after import |
| `--dry-run` | bool | Show what would be done |

---

## 6. Completion Command

### `completion`

Generate shell completion script.

```bash
liteboxd completion <shell>
```

Supported shells: `bash`, `zsh`, `fish`, `powershell`

**Installation**:
```bash
# Bash
liteboxd completion bash > /etc/bash_completion.d/liteboxd

# Zsh
liteboxd completion zsh > ~/.zfunc/_liteboxd

# Fish
liteboxd completion fish > ~/.config/fish/completions/liteboxd.fish
```

---

## 7. Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic error |
| 2 | Invalid usage (wrong arguments, flags) |
| 3 | API error (server returned error) |
| 4 | Not found |
| 5 | Conflict (resource already exists) |
| 6 | Timeout |
| 7 | Interrupted (Ctrl+C) |
| 128 | Command execution returned non-zero exit code |
