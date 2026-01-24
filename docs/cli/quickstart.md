# LiteBoxd CLI - Quick Start Guide

Get started with the LiteBoxd CLI in minutes.

---

## Installation

### From Binary (Recommended)

Download the latest release from GitHub:

```bash
# Linux (amd64)
wget https://github.com/fslongjin/liteboxd/releases/latest/download/liteboxd-linux-amd64 -O liteboxd
chmod +x liteboxd
sudo mv liteboxd /usr/local/bin/

# macOS (Intel)
wget https://github.com/fslongjin/liteboxd/releases/latest/download/liteboxd-darwin-amd64 -O liteboxd
chmod +x liteboxd
sudo mv liteboxd /usr/local/bin/

# macOS (Apple Silicon)
wget https://github.com/fslongjin/liteboxd/releases/latest/download/liteboxd-darwin-arm64 -O liteboxd
chmod +x liteboxd
sudo mv liteboxd /usr/local/bin/
```

### From Source

```bash
go install github.com/fslongjin/liteboxd/liteboxd-cli@latest
```

### Via Homebrew (macOS/Linux)

```bash
brew tap fslongjin/liteboxd
brew install liteboxd
```

---

## Configuration

Create a config file at `~/.config/liteboxd/config.yaml`:

```yaml
api-server: http://localhost:8080/api/v1
output: table
timeout: 30s

profiles:
  prod:
    api-server: https://liteboxd.example.com/api/v1
    token: your-token-here
```

---

## Sandbox Operations

### Create a Sandbox

**Important**: Sandbox creation is now template-only. You must create a template first (see Template Operations below).

```bash
# Create from template with defaults
liteboxd sandbox create --template python-data-science

# Create with overrides
liteboxd sandbox create --template python-data-science --ttl 7200 --env DEBUG=true

# Create from specific template version
liteboxd sandbox create --template python-ds --template-version 2

# Create and wait for ready
liteboxd sandbox create --template python-ds --wait
```

**Override Options**:
- `--cpu`: Override CPU limit
- `--memory`: Override memory limit
- `--ttl`: Override time to live
- `--env`: Override/merge environment variables

**Cannot Override** (come from template):
- Image
- Startup script
- Files
- Readiness probe

### List Sandboxes

```bash
# List all
liteboxd sandbox list

# Filter by status
liteboxd sandbox list --status running

# JSON output
liteboxd sandbox list --output json
```

### Get Sandbox Details

```bash
liteboxd sandbox get <sandbox-id>

# YAML output
liteboxd sandbox get <id> --output yaml
```

### Execute Commands

```bash
# Simple command
liteboxd sandbox exec <id> -- python -c "print('hello')"

# With arguments
liteboxd sandbox exec <id> -- npm install

# Long-running command
liteboxd sandbox exec <id> --timeout 5m -- npm test

# Only print stdout (quiet mode)
liteboxd sandbox exec <id> --quiet -- python script.py
```

### File Operations

```bash
# Upload file
liteboxd sandbox upload <id> ./main.py /workspace/main.py

# Download file
liteboxd sandbox download <id> /workspace/output.txt ./output.txt
```

### Logs

```bash
# Get logs
liteboxd sandbox logs <id>

# With events
liteboxd sandbox logs <id> --events

# Tail last 50 lines
liteboxd sandbox logs <id> --tail 50
```

### Wait for Ready

```bash
# Wait until sandbox is running
liteboxd sandbox wait <id>

# Custom timeout
liteboxd sandbox wait <id> --timeout 10m

# Quiet mode
liteboxd sandbox wait <id> --quiet
```

### Delete Sandbox

```bash
# Delete with confirmation
liteboxd sandbox delete <id>

# Force delete (no confirmation)
liteboxd sandbox delete <id> --force

# Wait for deletion
liteboxd sandbox delete <id> --wait
```

---

## Template Operations

### Create Template

```bash
# From YAML file
liteboxd template create --name python-ds --file template.yaml

# Using flags
liteboxd template create \
  --name node-basic \
  --image node:20-alpine \
  --cpu 500m --memory 512Mi \
  --tags node,javascript \
  --display-name "Node.js Basic" \
  --description "Basic Node.js environment"
```

### List Templates

```bash
# List all
liteboxd template list

# Filter by tag
liteboxd template list --tag python

# Search
liteboxd template list --search "data science"

# Pagination
liteboxd template list --page 2 --page-size 10
```

### Get Template

```bash
# Get latest version
liteboxd template get python-ds

# Get specific version
liteboxd template get python-ds --version 2

# JSON output
liteboxd template get python-ds --output json
```

### Update Template

```bash
# From YAML file
liteboxd template update python-ds \
  --file new-spec.yaml \
  --changelog "Add pandas library"

# Using flags
liteboxd template update python-ds \
  --image python:3.12-slim \
  --changelog "Upgrade to Python 3.12"
```

### Version Management

```bash
# List versions
liteboxd template versions python-ds

# Rollback
liteboxd template rollback python-ds --to 1

# With changelog
liteboxd template rollback python-ds --to 1 --changelog "Revert to 3.11"
```

### Export Template

```bash
# Export single template
liteboxd template export python-ds --output python-ds.yaml

# Export specific version
liteboxd template export python-ds --version 1 --output python-ds-v1.yaml

# Export all templates
liteboxd template export --output all-templates.yaml

# Export filtered by tag
liteboxd template export --tag python --output python-templates.yaml

# Export specific templates
liteboxd template export --names "python-ds,node-basic" --output selected.yaml
```

### Delete Template

```bash
# Delete with confirmation
liteboxd template delete python-ds

# Force delete
liteboxd template delete python-ds --force
```

---

## Import/Export

### Import Templates

```bash
# Import with create-or-update strategy
liteboxd import --file templates.yaml

# Import with auto-prepull
liteboxd import --file templates.yaml --prepull

# Create only
liteboxd import --file templates.yaml --strategy create-only

# Dry run
liteboxd import --file templates.yaml --dry-run
```

---

## Image Prepull

### Trigger Prepull

```bash
# Prepull an image
liteboxd image prepull python:3.11-slim

# Prepull template image
liteboxd image prepull --template python-ds

# Wait for completion
liteboxd image prepull python:3.11-slim --wait

# Custom timeout
liteboxd image prepull python:3.11-slim --timeout 20m
```

### List Prepull Tasks

```bash
# List all
liteboxd image list

# Filter by image
liteboxd image list --image python:3.11-slim

# Filter by status
liteboxd image list --status completed

# JSON output
liteboxd image list --output json
```

### Delete Prepull Task

```bash
liteboxd image delete <task-id>
```

---

## Shell Completion

### Install Completion

```bash
# Bash
liteboxd completion bash > /etc/bash_completion.d/liteboxd
source /etc/bash_completion.d/liteboxd

# Zsh
liteboxd completion zsh > ~/.zfunc/_liteboxd
# Add to ~/.zshrc: fpath+=~/.zfunc

# Fish
liteboxd completion fish > ~/.config/fish/completions/liteboxd.fish
```

---

## Common Patterns

### CI/CD Pipeline

```bash
#!/bin/bash
set -e

# Create test environment from template
SANDBOX_ID=$(liteboxd sandbox create --template test-env --quiet)

# Cleanup on exit
trap "liteboxd sandbox delete $SANDBOX_ID --force" EXIT

# Wait for ready
liteboxd sandbox wait $SANDBOX_ID

# Run tests
liteboxd sandbox exec $SANDBOX_ID -- npm test

# Upload results
liteboxd sandbox download $SANDBOX_ID /workspace/test-results.xml ./test-results.xml
```

### Quick Script Execution

```bash
# Create sandbox from template
ID=$(liteboxd sandbox create --template python-ds --quiet)

# Wait and run
liteboxd sandbox wait $ID
liteboxd sandbox exec $ID -- python -c "
import numpy as np
print(np.array([1, 2, 3]))
"

# Cleanup
liteboxd sandbox delete $ID --force
```

### Script-Friendly Output

```bash
# Get just the sandbox ID
ID=$(liteboxd sandbox create --template python-ds --quiet)

# Get exit code only
liteboxd sandbox exec $ID --quiet --exit-code -- python script.py
echo "Exit code: $?"
```

---

## Next Steps

- See [design.md](design.md) for detailed architecture
- See [command-reference.md](command-reference.md) for complete command reference
- Use `--help` flag on any command for usage information
