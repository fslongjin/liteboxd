# LiteBoxd

A lightweight K8s sandbox system inspired by e2b, designed to run on k3s.

## Features

- Sandbox lifecycle management (create, list, get, delete)
- Command execution in sandboxes
- File upload/download
- Automatic TTL-based cleanup
- Web UI for easy management
- **Network isolation with Cilium** (default deny-all egress)
- **Token-based sandbox access** via gateway service
- **Configurable internet access** per template

## Quick Start

### Prerequisites

```bash
cd backend && go mod tidy
```

### 1. Prepare Remote K3s + Cilium

在独立机器上部署 K3s 和 Cilium，并确保本机可以访问该集群。
要求不要在 Docker 容器内安装 K3s。

参考文档：
https://docs.cilium.io/en/stable/installation/k3s/

### 2. Configure Kubeconfig

将远程机器上的 kubeconfig 拷贝到本机，并设置环境变量：

```bash
export KUBECONFIG=~/.kube/config
```

### 3. Start Backend

```bash
make run-backend
```

The API server will start on `http://localhost:8080`.

### 4. Start Gateway

```bash
make run-gateway
```

The gateway server will start on `http://localhost:8081`.

### 5. Start Frontend

```bash
make run-frontend
```

The web UI will be available at `http://localhost:3000`.

### All-in-One

Run all services:

```bash
make run-all
```

> **Tip**: Run `make help` to see all available commands.
> **See also**: [Network Access Guide](docs/network-access-guide.md) for network feature documentation.

## Security

- Pods run as non-root user (UID 1000)
- Privilege escalation is disabled
- Resource limits prevent resource exhaustion
- All sandboxes run in dedicated namespace
- Seccomp profile enabled
- **Default-deny network policies** (Cilium)
- **Token-based authentication** for sandbox access
- **Sandbox isolation from K8s API Server**

## License

[GPL-3.0](LICENSE)
