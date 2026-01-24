# LiteBoxd Go SDK

The official Go SDK for LiteBoxd - a lightweight Kubernetes-based sandbox system.

## Installation

```bash
go get github.com/fslongjin/liteboxd/sdk/go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    liteboxd "github.com/fslongjin/liteboxd/sdk/go"
)

func main() {
    // Create a client
    client := liteboxd.NewClient(
        "http://localhost:8080/api/v1",
        liteboxd.WithTimeout(30*time.Second),
    )

    ctx := context.Background()

    // Create a sandbox from template
    sandbox, err := client.Sandbox.Create(ctx, "python-data-science", nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created sandbox: %s\n", sandbox.ID)

    // Wait for sandbox to be ready
    sandbox, err = client.Sandbox.WaitForReady(ctx, sandbox.ID, 2*time.Second, 5*time.Minute)
    if err != nil {
        log.Fatal(err)
    }

    // Execute a command
    resp, err := client.Sandbox.Execute(ctx, sandbox.ID, []string{"python", "--version"}, 30)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(resp.Stdout)

    // Clean up
    client.Sandbox.Delete(ctx, sandbox.ID)
}
```

## Project Structure

```
sdk/go/
├── client.go          # Main client
├── options.go         # Client options
├── errors.go          # Error handling
├── sandbox.go         # Sandbox service
├── template.go        # Template service
├── prepull.go         # Prepull service
├── import_export.go   # Import/Export service
└── types*.go          # Data types
```

## Documentation

- [Design Document](../../docs/sdk/go/design.md)
- [API Reference](../../docs/sdk/go/api-reference.md)
- [Quick Start Guide](../../docs/sdk/go/quickstart.md)

## License

GPL-3.0
