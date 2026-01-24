# LiteBoxd CLI Design Document

## 1. Overview

This document describes the design and implementation plan for the LiteBoxd CLI tool.

### 1.1 Goals

- Provide a powerful command-line interface for LiteBoxd
- Support template-centric workflow (all sandboxes created from templates)
- Offer human-readable and machine-parseable output formats
- Enable shell completion and scripting

### 1.2 Non-Goals

- Interactive shell mode (future feature)
- Built-in text editor for template editing (use external editors)
- Direct sandbox creation without template (not supported by API)

---

## 2. Project Structure

```
liteboxd-cli/
├── go.mod
├── go.sum
├── README.md
├── main.go                   # Entry point, command registration
├── cmd/
│   ├── root.go               # Root command
│   ├── sandbox.go            # Sandbox commands
│   ├── template.go           # Template commands
│   ├── prepull.go            # Prepull commands
│   └── import_export.go      # Import/Export commands
├── internal/
│   ├── config/
│   │   └── config.go         # CLI configuration (profile, output format)
│   ├── output/
│   │   ├── formatter.go      # Output formatting interface
│   │   ├── table.go          # Table formatter
│   │   ├── json.go           # JSON formatter
│   │   └── yaml.go           # YAML formatter
│   └── utils/
│       └── spinner.go        # Loading spinner for async operations
└── README.md
```

**Module Path**: `github.com/fslongjin/liteboxd/liteboxd-cli`

**Binary Name**: `liteboxd`

---

## 3. CLI Framework

**Choice**: [Cobra](https://github.com/spf13/cobra)

**Rationale**:
- Industry standard for Go CLIs (kubectl, docker CLI, etc.)
- Built-in support for subcommands, flags, help generation
- Good integration with [Viper](https://github.com/spf13/viper) for configuration

---

## 4. Command Structure

```
liteboxd [global flags] <command> [subcommand] [flags] [args]

Global Flags:
  --api-server string    API server address (default: http://localhost:8080/api/v1)
  --output string        Output format: table, json, yaml (default: table)
  --timeout duration     Request timeout (default: 30s)
  --verbose              Enable verbose output
  --config string        Config file path (default: ~/.config/liteboxd/config.yaml)

Commands:
  sandbox        Manage sandboxes
  template       Manage templates
  image          Manage image prepull
  import         Import templates from YAML
  completion     Generate shell completion
```

---

## 5. Output Formats

### Table Format (default)

```
ID           IMAGE              STATUS    CREATED           EXPIRES
a1b2c3d4     python:3.11-slim   running   2 minutes ago     in 58 minutes
e5f6g7h8     node:20-alpine     pending   5 seconds ago     in 59 minutes
```

### JSON Format

Machine-readable, full object structure:
```json
{"items":[{"id":"a1b2c3d4","image":"python:3.11-slim",...}]}
```

### YAML Format

Human-readable structured data:
```yaml
items:
  - id: a1b2c3d4
    image: python:3.11-slim
    ...
```

---

## 6. Configuration File

`~/.config/liteboxd/config.yaml`:

```yaml
# Default API server
api-server: http://localhost:8080/api/v1

# Default output format
output: table

# Default timeout
timeout: 30s

# Profiles for multiple environments
profiles:
  production:
    api-server: https://liteboxd.example.com/api/v1
    token: your-api-token
  staging:
    api-server: https://liteboxd-staging.example.com/api/v1
```

---

## 7. Implementation Plan

### Phase 1: CLI Foundation

| Task | Description |
|------|-------------|
| 1.1 | Initialize CLI module with Cobra |
| 1.2 | Implement root command and global flags |
| 1.3 | Implement configuration file handling |
| 1.4 | Implement output formatters (table, json, yaml) |
| 1.5 | Add shell completion support |

### Phase 2: Sandbox Commands

| Task | Description |
|------|-------------|
| 2.1 | Implement `sandbox create` command |
| 2.2 | Implement `sandbox list` and `sandbox get` commands |
| 2.3 | Implement `sandbox delete` command |
| 2.4 | Implement `sandbox exec` command |
| 2.5 | Implement `sandbox logs` command |
| 2.6 | Implement `sandbox upload/download` commands |
| 2.7 | Implement `sandbox wait` command |

### Phase 3: Template Commands

| Task | Description |
|------|-------------|
| 3.1 | Implement `template create` command |
| 3.2 | Implement `template list` and `template get` commands |
| 3.3 | Implement `template update` and `template delete` commands |
| 3.4 | Implement `template versions` command |
| 3.5 | Implement `template rollback` command |
| 3.6 | Implement `template export` command |

### Phase 4: Import/Export & Prepull Commands

| Task | Description |
|------|-------------|
| 4.1 | Implement `import` command |
| 4.2 | Implement `image prepull` command |
| 4.3 | Implement `image list` and `image delete` commands |

### Phase 5: Documentation

| Task | Description |
|------|-------------|
| 5.1 | Write CLI README |
| 5.2 | Add usage examples |

---

## 8. Dependencies

```go
require (
    github.com/fslongjin/liteboxd/sdk/go
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.0
    github.com/olekukonko/tablewriter // For table output
)
```

---

## 9. Design Decisions

1. **Table Output Default**: Human-readable table format is default for list commands
2. **Quiet Flag**: `-q` flag for minimal output (useful in scripts)
3. **Wait Flag**: `--wait` flag for async operations (sandbox create, prepull)
4. **Config File**: XDG-compliant config file location
5. **Profiles**: Support multiple environment profiles (dev, staging, prod)

---

## 10. Open Questions

1. **Binary Distribution**: How should the CLI be distributed?
   - **Options**: GitHub releases, Homebrew tap, Go install
   - **Recommendation**: Support all three

2. **Short Alias**: Should we support `lbox` as a short alias?
   - **Recommendation**: Yes, create a symlink in installation docs

---

## 11. Success Criteria

- [ ] CLI supports all common user workflows
- [ ] CLI output is both human-readable and machine-parseable
- [ ] Shell completion works for bash, zsh, fish
- [ ] Documentation includes usage examples for all commands
- [ ] CLI can be built and installed with standard Go tooling
