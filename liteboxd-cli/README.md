# LiteBoxd CLI

Command-line interface for LiteBoxd - a lightweight Kubernetes-based sandbox system.

## Installation

### From Binary

Download the latest release from GitHub:

```bash
wget https://github.com/fslongjin/liteboxd/releases/latest/download/liteboxd-linux-amd64 -O liteboxd
chmod +x liteboxd
sudo mv liteboxd /usr/local/bin/
```

### From Source

```bash
go install github.com/fslongjin/liteboxd/liteboxd-cli@latest
```

## Quick Start

```bash
# Create a sandbox from template
liteboxd sandbox create --template python-data-science

# List sandboxes
liteboxd sandbox list

# Execute command
liteboxd sandbox exec <id> -- python --version

# Delete sandbox
liteboxd sandbox delete <id>
```

## Configuration

Create a config file at `~/.config/liteboxd/config.yaml`:

```yaml
api-server: http://localhost:8080/api/v1
output: table
timeout: 30s
```

## Commands

### Sandbox Commands

```bash
liteboxd sandbox create --template <name> [flags]
liteboxd sandbox list [--output table|json|yaml]
liteboxd sandbox get <id>
liteboxd sandbox delete <id>
liteboxd sandbox exec <id> -- <command> [args...]
liteboxd sandbox logs <id>
liteboxd sandbox upload <id> <local-path> <remote-path>
liteboxd sandbox download <id> <remote-path> [local-path]
liteboxd sandbox wait <id>
```

### Template Commands

```bash
liteboxd template create --name <name> --file <yaml-file>
liteboxd template list [--tag <tag>]
liteboxd template get <name>
liteboxd template update <name> --file <yaml-file>
liteboxd template delete <name>
liteboxd template versions <name>
liteboxd template rollback <name> --to <version>
liteboxd template export [name] [--output <file>]
```

### Image Commands

```bash
liteboxd image prepull <image> [--template <name>]
liteboxd image list
liteboxd image delete <id>
```

### Import Command

```bash
liteboxd import --file <yaml-file> [--strategy create-or-update] [--prepull]
```

## Documentation

- [Design Document](../../docs/cli/design.md)
- [Command Reference](../../docs/cli/command-reference.md)
- [Quick Start Guide](../../docs/cli/quickstart.md)

## License

GPL-3.0
