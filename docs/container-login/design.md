# Container Login (Interactive Exec) Feature Design

## 1. Overview

### 1.1 Goal

Extend the existing `exec` 能力, 支持交互式终端会话, 使用户可以通过 Web UI / CLI / SDK 三种方式"登录"沙箱容器.

### 1.2 User Stories

| 入口 | 操作 | 底层能力 |
|------|------|----------|
| Web UI | 沙箱详情页 -> 点击"登录沙箱"按钮 -> 弹出全屏终端 | WebSocket exec |
| CLI | `liteboxd sandbox exec -it <id> /bin/bash` | WebSocket exec |
| SDK | `client.Sandbox.ExecInteractive(ctx, id, cmd, opts)` | WebSocket exec |

### 1.3 Design Principles

- **Exec 是底层原语**: "登录沙箱" 本质上就是 `exec` + TTY + stdin 流式传输. 不引入独立的 "terminal" 或 "login" 概念.
- **统一协议**: Web / CLI / SDK 均使用同一个 WebSocket 端点, 统一通信协议.
- **最小侵入**: 现有的非交互式 `POST /exec` API 保持不变, 新增 WebSocket 端点.

---

## 2. Architecture

### 2.1 Overall Data Flow

```
                   WebSocket
  [Web xterm.js] ──────────┐
                            │
                   WebSocket│
  [CLI terminal] ──────────┤──> [API Server] ──SPDY──> [K8s API] ──> [Pod Container]
                            │   (ws handler)           (exec)
                   WebSocket│
  [SDK client]   ──────────┘
```

### 2.2 Component Changes

| Component | Changes |
|-----------|---------|
| `backend/internal/k8s/client.go` | New: `ExecInteractive()` method with streaming I/O |
| `backend/internal/handler/sandbox.go` | New: `ExecInteractive()` WebSocket handler |
| `backend/internal/service/sandbox.go` | New: `ExecInteractive()` service method |
| `backend/internal/model/sandbox.go` | New: message types for WebSocket protocol |
| `backend/cmd/server/main.go` | Register new route |
| `web/src/views/SandboxDetail.vue` | New: "登录沙箱" button + terminal dialog |
| `web/src/components/TerminalDialog.vue` | New: xterm.js terminal component |
| `web/src/api/sandbox.ts` | New: WebSocket URL builder |
| `web/package.json` | New deps: `@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-web-links` |
| `liteboxd-cli/cmd/sandbox.go` | Extend: `exec` command 支持 `-i`, `-t` flags |
| `sdk/go/sandbox.go` | New: `ExecInteractive()` method |
| `sdk/go/types.go` | New: `ExecInteractiveRequest`, `ExecSession` types |

---

## 3. WebSocket Protocol Design

### 3.1 Endpoint

```
GET /api/v1/sandboxes/:id/exec/interactive
```

Upgrade to WebSocket. Query parameters:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `command` | string (repeated) | `["sh"]` | Command and arguments. Multiple `command` params for args. |
| `tty` | bool | `true` | Allocate pseudo-TTY |
| `rows` | int | `24` | Initial terminal rows |
| `cols` | int | `80` | Initial terminal columns |

Example:

```
ws://localhost:8080/api/v1/sandboxes/abc123/exec/interactive?command=bash&tty=true&cols=120&rows=40
```

### 3.2 Message Protocol

All messages are JSON text frames.

**Client -> Server:**

```jsonc
// Terminal input (keyboard data)
{"type": "input", "data": "ls -la\r"}

// Terminal resize
{"type": "resize", "cols": 120, "rows": 40}
```

**Server -> Client:**

```jsonc
// Terminal output
{"type": "output", "data": "total 42\r\ndrwxr-xr-x  2 user user 4096 ...\r\n"}

// Process exited
{"type": "exit", "exitCode": 0}

// Error
{"type": "error", "message": "container not running"}
```

### 3.3 Lifecycle

```
Client                          Server
  |                                |
  |──── WebSocket Upgrade ────────>|
  |                                |── Create K8s SPDY exec session
  |<─── Connection Accepted ──────|
  |                                |
  |──── {type:input} ────────────>|── Forward to container stdin
  |<─── {type:output} ───────────|<── Read from container stdout/stderr
  |                                |
  |──── {type:resize} ───────────>|── TerminalSizeQueue.Next()
  |                                |
  |     ... interactive session ...|
  |                                |
  |<─── {type:exit, exitCode:0} ──|── Container process exits
  |──── WebSocket Close ─────────>|
  |                                |
```

---

## 4. Backend Implementation

### 4.1 K8s Client: ExecInteractive

File: `backend/internal/k8s/client.go`

```go
// ExecInteractiveOptions defines options for interactive exec
type ExecInteractiveOptions struct {
    Command []string
    TTY     bool
    Stdin   io.Reader
    Stdout  io.Writer
    Stderr  io.Writer  // nil when TTY=true (merged into stdout)
    // TerminalSizeQueue provides terminal resize events.
    // Implements remotecommand.TerminalSizeQueue.
    TerminalSizeQueue remotecommand.TerminalSizeQueue
}

// ExecInteractive executes a command interactively with streaming I/O.
// This is the core primitive - stdin/stdout/stderr are streams, not buffers.
// Blocks until the command exits or ctx is cancelled.
func (c *Client) ExecInteractive(ctx context.Context, sandboxID string, opts ExecInteractiveOptions) error {
    podName := fmt.Sprintf("sandbox-%s", sandboxID)

    req := c.clientset.CoreV1().RESTClient().Post().
        Resource("pods").
        Name(podName).
        Namespace(SandboxNamespace).
        SubResource("exec").
        VersionedParams(&corev1.PodExecOptions{
            Container: "main",
            Command:   opts.Command,
            Stdin:     opts.Stdin != nil,
            Stdout:    true,
            Stderr:    !opts.TTY,  // stderr merges into stdout when TTY
            TTY:       opts.TTY,
        }, scheme.ParameterCodec)

    exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
    if err != nil {
        return fmt.Errorf("failed to create executor: %w", err)
    }

    streamOpts := remotecommand.StreamOptions{
        Stdin:             opts.Stdin,
        Stdout:            opts.Stdout,
        Stderr:            opts.Stderr,
        Tty:               opts.TTY,
        TerminalSizeQueue: opts.TerminalSizeQueue,
    }

    return exec.StreamWithContext(ctx, streamOpts)
}
```

### 4.2 Terminal Size Queue

File: `backend/internal/k8s/terminal.go` (new)

```go
// TerminalSize represents terminal dimensions
type TerminalSize struct {
    Width  uint16
    Height uint16
}

// SizeQueue implements remotecommand.TerminalSizeQueue
// for receiving terminal resize events from WebSocket clients.
type SizeQueue struct {
    resizeChan chan remotecommand.TerminalSize
}

func NewSizeQueue() *SizeQueue {
    return &SizeQueue{
        resizeChan: make(chan remotecommand.TerminalSize, 1),
    }
}

// Next returns the next terminal size. Blocks until a resize event is available.
// Returns nil when the queue is closed (session ended).
func (sq *SizeQueue) Next() *remotecommand.TerminalSize {
    size, ok := <-sq.resizeChan
    if !ok {
        return nil
    }
    return &size
}

// Push sends a resize event to the queue. Non-blocking; drops old events.
func (sq *SizeQueue) Push(width, height uint16) {
    select {
    case sq.resizeChan <- remotecommand.TerminalSize{Width: width, Height: height}:
    default:
        // Drop old event and push new one
        select {
        case <-sq.resizeChan:
        default:
        }
        sq.resizeChan <- remotecommand.TerminalSize{Width: width, Height: height}
    }
}

// Close closes the size queue.
func (sq *SizeQueue) Close() {
    close(sq.resizeChan)
}
```

### 4.3 WebSocket Handler

File: `backend/internal/handler/sandbox.go`

New route registration:

```go
func (h *SandboxHandler) RegisterRoutes(r *gin.RouterGroup) {
    sandboxes := r.Group("/sandboxes")
    {
        // ... existing routes ...
        sandboxes.GET("/:id/exec/interactive", h.ExecInteractive) // NEW
    }
}
```

Handler implementation:

```go
func (h *SandboxHandler) ExecInteractive(c *gin.Context) {
    id := c.Param("id")

    // Parse query parameters
    command := c.QueryArray("command")
    if len(command) == 0 {
        command = []string{"sh"}
    }
    tty := c.DefaultQuery("tty", "true") == "true"
    rows, _ := strconv.Atoi(c.DefaultQuery("rows", "24"))
    cols, _ := strconv.Atoi(c.DefaultQuery("cols", "80"))

    // Upgrade to WebSocket
    upgrader := websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool { return true }, // CORS handled by middleware
        ReadBufferSize:  1024,
        WriteBufferSize: 1024,
    }

    ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upgrade to websocket"})
        return
    }
    defer ws.Close()

    // Bridge WebSocket to K8s exec
    h.svc.ExecInteractive(c.Request.Context(), ws, id, command, tty, rows, cols)
}
```

### 4.4 Service: ExecInteractive

File: `backend/internal/service/sandbox.go`

```go
func (s *SandboxService) ExecInteractive(ctx context.Context, ws *websocket.Conn, id string, command []string, tty bool, rows, cols int) {
    // Create terminal size queue
    sizeQueue := k8s.NewSizeQueue()
    defer sizeQueue.Close()

    // Push initial size
    sizeQueue.Push(uint16(cols), uint16(rows))

    // Create pipes for stdin
    stdinReader, stdinWriter := io.Pipe()
    defer stdinWriter.Close()

    // Create a writer that sends output to WebSocket
    wsWriter := &wsOutputWriter{ws: ws}

    // Read WebSocket messages in a goroutine (stdin + resize)
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    go func() {
        defer stdinWriter.Close()
        for {
            _, message, err := ws.ReadMessage()
            if err != nil {
                cancel()
                return
            }

            var msg WSMessage
            if err := json.Unmarshal(message, &msg); err != nil {
                continue
            }

            switch msg.Type {
            case "input":
                stdinWriter.Write([]byte(msg.Data))
            case "resize":
                sizeQueue.Push(uint16(msg.Cols), uint16(msg.Rows))
            }
        }
    }()

    // Run interactive exec (blocks until process exits)
    err := s.k8sClient.ExecInteractive(ctx, id, k8s.ExecInteractiveOptions{
        Command:           command,
        TTY:               tty,
        Stdin:             stdinReader,
        Stdout:            wsWriter,
        Stderr:            nil, // merged into stdout when TTY
        TerminalSizeQueue: sizeQueue,
    })

    // Send exit message
    exitCode := 0
    if err != nil {
        if exitErr, ok := err.(interface{ ExitStatus() int }); ok {
            exitCode = exitErr.ExitStatus()
        } else {
            exitCode = 1
        }
    }

    exitMsg, _ := json.Marshal(WSMessage{Type: "exit", ExitCode: exitCode})
    ws.WriteMessage(websocket.TextMessage, exitMsg)
}
```

### 4.5 Model: WebSocket Message Types

File: `backend/internal/model/sandbox.go` (additions)

```go
// WSMessage represents a WebSocket message for interactive exec
type WSMessage struct {
    Type     string `json:"type"`               // "input", "output", "resize", "exit", "error"
    Data     string `json:"data,omitempty"`      // terminal data (input/output)
    Cols     int    `json:"cols,omitempty"`       // terminal columns (resize)
    Rows     int    `json:"rows,omitempty"`       // terminal rows (resize)
    ExitCode int    `json:"exitCode,omitempty"`   // process exit code (exit)
    Message  string `json:"message,omitempty"`    // error message (error)
}
```

### 4.6 WebSocket Output Writer

包装 `*websocket.Conn`, 实现 `io.Writer`, 将 K8s exec 的 stdout 转发为 WebSocket output 消息:

```go
// wsOutputWriter wraps a WebSocket connection as an io.Writer
// Sends terminal output as JSON messages to the client
type wsOutputWriter struct {
    ws  *websocket.Conn
    mu  sync.Mutex
}

func (w *wsOutputWriter) Write(p []byte) (int, error) {
    w.mu.Lock()
    defer w.mu.Unlock()

    msg, _ := json.Marshal(WSMessage{
        Type: "output",
        Data: string(p),
    })
    err := w.ws.WriteMessage(websocket.TextMessage, msg)
    if err != nil {
        return 0, err
    }
    return len(p), nil
}
```

### 4.7 Dependencies (Backend)

```
go get github.com/gorilla/websocket
```

---

## 5. Web UI Implementation

### 5.1 New Dependencies

```bash
cd web
npm install @xterm/xterm @xterm/addon-fit @xterm/addon-web-links
```

### 5.2 New Component: TerminalDialog.vue

File: `web/src/components/TerminalDialog.vue`

核心逻辑:

```vue
<template>
  <t-dialog
    v-model:visible="visible"
    header="登录沙箱"
    :footer="false"
    width="80%"
    placement="center"
    @close="disconnect"
    :close-on-overlay-click="false"
  >
    <div ref="terminalRef" class="terminal-container"></div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted, nextTick } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

const props = defineProps<{
  sandboxId: string
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
}>()

const terminalRef = ref<HTMLElement>()
let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null

function connect() {
  if (!terminalRef.value) return

  // Create terminal
  terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#d4d4d4',
    },
  })

  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(new WebLinksAddon())

  terminal.open(terminalRef.value)
  fitAddon.fit()

  const { cols, rows } = terminal

  // Connect WebSocket
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const baseUrl = import.meta.env.VITE_API_URL || `${location.host}/api/v1`
  const wsUrl = `${protocol}//${baseUrl}/sandboxes/${props.sandboxId}/exec/interactive?command=sh&tty=true&cols=${cols}&rows=${rows}`

  ws = new WebSocket(wsUrl)

  ws.onopen = () => {
    terminal?.writeln('Connected to sandbox.\r\n')
  }

  ws.onmessage = (event) => {
    const msg = JSON.parse(event.data)
    switch (msg.type) {
      case 'output':
        terminal?.write(msg.data)
        break
      case 'exit':
        terminal?.writeln(`\r\nProcess exited with code ${msg.exitCode}`)
        break
      case 'error':
        terminal?.writeln(`\r\nError: ${msg.message}`)
        break
    }
  }

  ws.onclose = () => {
    terminal?.writeln('\r\nConnection closed.')
  }

  // Forward terminal input to WebSocket
  terminal.onData((data) => {
    ws?.send(JSON.stringify({ type: 'input', data }))
  })

  // Handle terminal resize
  terminal.onResize(({ cols, rows }) => {
    ws?.send(JSON.stringify({ type: 'resize', cols, rows }))
  })

  // Handle window resize
  window.addEventListener('resize', handleResize)
}

function handleResize() {
  fitAddon?.fit()
}

function disconnect() {
  window.removeEventListener('resize', handleResize)
  ws?.close()
  ws = null
  terminal?.dispose()
  terminal = null
  fitAddon = null
  emit('update:visible', false)
}

watch(() => props.visible, (val) => {
  if (val) {
    nextTick(() => connect())
  } else {
    disconnect()
  }
})

onUnmounted(() => {
  disconnect()
})
</script>

<style scoped>
.terminal-container {
  height: 500px;
  background: #1e1e1e;
  padding: 4px;
}
</style>
```

### 5.3 SandboxDetail.vue Changes

在右上角的 actions 区域增加"登录沙箱"按钮:

```vue
<template #actions>
  <t-space>
    <t-button theme="primary" @click="showTerminal = true"
      :disabled="sandbox?.status !== 'running'">
      登录沙箱
    </t-button>
    <t-popconfirm content="确定要删除该 Sandbox 吗？" @confirm="deleteSandbox">
      <t-button theme="danger" variant="outline">删除</t-button>
    </t-popconfirm>
  </t-space>
</template>

<!-- Terminal dialog -->
<TerminalDialog
  v-model:visible="showTerminal"
  :sandbox-id="sandboxId"
/>
```

新增状态:

```typescript
const showTerminal = ref(false)
```

### 5.4 API URL Builder

File: `web/src/api/sandbox.ts` (additions)

```typescript
export const sandboxApi = {
  // ... existing methods ...

  // Build WebSocket URL for interactive exec
  getExecWsUrl: (id: string, options?: {
    command?: string,
    tty?: boolean,
    cols?: number,
    rows?: number,
  }) => {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const base = import.meta.env.VITE_WS_URL || `${location.host}/api/v1`
    const params = new URLSearchParams()
    params.set('command', options?.command || 'sh')
    params.set('tty', String(options?.tty ?? true))
    params.set('cols', String(options?.cols ?? 80))
    params.set('rows', String(options?.rows ?? 24))
    return `${protocol}//${base}/sandboxes/${id}/exec/interactive?${params.toString()}`
  },
}
```

---

## 6. CLI Implementation

### 6.1 Extended `exec` Command

File: `liteboxd-cli/cmd/sandbox.go`

扩展 `exec` 命令, 增加 `-i` (stdin) 和 `-t` (tty) flags:

```go
var (
    execInteractive bool // -i flag
    execTTY         bool // -t flag
)

var sandboxExecCmd = &cobra.Command{
    Use:   "exec <id> [flags] -- <command> [args...]",
    Short: "Execute command in sandbox",
    Args:  cobra.MinimumNArgs(1),
    Example: `  # Non-interactive (existing behavior)
  liteboxd sandbox exec <id> -- python -c "print('hello')"

  # Interactive shell login
  liteboxd sandbox exec -it <id> -- /bin/bash

  # Short form: login with default shell
  liteboxd sandbox exec -it <id>`,
    RunE: runSandboxExec,
}

func init() {
    // ... existing flags ...
    sandboxExecCmd.Flags().BoolVarP(&execInteractive, "stdin", "i", false, "Pass stdin to the container")
    sandboxExecCmd.Flags().BoolVarP(&execTTY, "tty", "t", false, "Allocate a pseudo-TTY")
}
```

### 6.2 Interactive Exec Runner

当 `-i` 或 `-t` flag 存在时, 走交互式路径:

```go
func runSandboxExec(cmd *cobra.Command, args []string) error {
    id := args[0]
    cmdArgs := args[1:]

    // If -i or -t, use interactive mode
    if execInteractive || execTTY {
        return runInteractiveExec(id, cmdArgs)
    }

    // Existing non-interactive path ...
    return runNonInteractiveExec(cmd, id, cmdArgs)
}

func runInteractiveExec(id string, command []string) error {
    if len(command) == 0 {
        command = []string{"sh"} // default shell
    }

    client := getAPIClient()

    // Get terminal size
    cols, rows := getTerminalSize() // uses golang.org/x/term

    // Connect via SDK
    session, err := client.Sandbox.ExecInteractive(context.Background(), id, &liteboxd.ExecInteractiveRequest{
        Command: command,
        TTY:     execTTY,
        Cols:    cols,
        Rows:    rows,
    })
    if err != nil {
        return fmt.Errorf("failed to start interactive session: %w", err)
    }
    defer session.Close()

    // Set terminal to raw mode if TTY
    if execTTY {
        oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
        if err != nil {
            return fmt.Errorf("failed to set raw mode: %w", err)
        }
        defer term.Restore(int(os.Stdin.Fd()), oldState)
    }

    // Handle terminal resize signals (SIGWINCH)
    go watchTerminalResize(session)

    // Bidirectional copy
    errCh := make(chan error, 2)

    // stdin -> session
    go func() {
        _, err := io.Copy(session, os.Stdin)
        errCh <- err
    }()

    // session -> stdout
    go func() {
        _, err := io.Copy(os.Stdout, session)
        errCh <- err
    }()

    // Wait for session to end
    exitCode := session.Wait()

    if exitCode != 0 {
        os.Exit(exitCode)
    }
    return nil
}
```

### 6.3 CLI Dependencies

```
go get golang.org/x/term
go get github.com/gorilla/websocket
```

---

## 7. SDK Implementation

### 7.1 New Types

File: `sdk/go/types.go` (additions)

```go
// ExecInteractiveRequest defines parameters for interactive exec
type ExecInteractiveRequest struct {
    Command []string `json:"command"`
    TTY     bool     `json:"tty"`
    Cols    int      `json:"cols"`
    Rows    int      `json:"rows"`
}
```

File: `backend/pkg/model/sandbox.go` (additions)

```go
// ExecInteractiveRequest defines parameters for interactive exec
type ExecInteractiveRequest struct {
    Command []string `json:"command"`
    TTY     bool     `json:"tty"`
    Cols    int      `json:"cols"`
    Rows    int      `json:"rows"`
}

// WSMessage represents a WebSocket message for interactive exec
type WSMessage struct {
    Type     string `json:"type"`
    Data     string `json:"data,omitempty"`
    Cols     int    `json:"cols,omitempty"`
    Rows     int    `json:"rows,omitempty"`
    ExitCode int    `json:"exitCode,omitempty"`
    Message  string `json:"message,omitempty"`
}
```

### 7.2 ExecSession

File: `sdk/go/exec_session.go` (new)

`ExecSession` wraps a WebSocket connection, 实现 `io.Reader`, `io.Writer`, 提供 `Resize()` 和 `Wait()`:

```go
// ExecSession represents an interactive exec session.
// Implements io.Reader (stdout) and io.Writer (stdin).
type ExecSession struct {
    ws       *websocket.Conn
    readBuf  bytes.Buffer
    readMu   sync.Mutex
    readCh   chan struct{}
    exitCode int
    exitCh   chan struct{}
    closed   bool
}

// Read reads stdout data from the session.
// Implements io.Reader.
func (s *ExecSession) Read(p []byte) (int, error) { ... }

// Write sends stdin data to the session.
// Implements io.Writer.
func (s *ExecSession) Write(p []byte) (int, error) {
    msg, _ := json.Marshal(WSMessage{Type: "input", Data: string(p)})
    return len(p), s.ws.WriteMessage(websocket.TextMessage, msg)
}

// Resize sends a terminal resize event.
func (s *ExecSession) Resize(cols, rows int) error {
    msg, _ := json.Marshal(WSMessage{Type: "resize", Cols: cols, Rows: rows})
    return s.ws.WriteMessage(websocket.TextMessage, msg)
}

// Wait blocks until the session ends. Returns the exit code.
func (s *ExecSession) Wait() int {
    <-s.exitCh
    return s.exitCode
}

// Close closes the session.
func (s *ExecSession) Close() error {
    return s.ws.Close()
}
```

### 7.3 SandboxService.ExecInteractive

File: `sdk/go/sandbox.go` (additions)

```go
// ExecInteractive starts an interactive exec session via WebSocket.
// Returns an ExecSession that implements io.Reader (stdout) and io.Writer (stdin).
func (s *SandboxService) ExecInteractive(ctx context.Context, id string, req *ExecInteractiveRequest) (*ExecSession, error) {
    // Build WebSocket URL
    u := *s.client.baseURL
    u.Scheme = "ws"
    if s.client.baseURL.Scheme == "https" {
        u.Scheme = "wss"
    }
    u.Path = s.client.baseURL.Path + "/" + s.client.buildPath("sandboxes", id, "exec", "interactive")

    q := u.Query()
    for _, cmd := range req.Command {
        q.Add("command", cmd)
    }
    q.Set("tty", fmt.Sprintf("%v", req.TTY))
    q.Set("rows", fmt.Sprintf("%d", req.Rows))
    q.Set("cols", fmt.Sprintf("%d", req.Cols))
    u.RawQuery = q.Encode()

    // WebSocket dial
    header := http.Header{}
    if s.client.authToken != "" {
        header.Set("Authorization", "Bearer "+s.client.authToken)
    }

    ws, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
    if err != nil {
        return nil, fmt.Errorf("failed to connect: %w", err)
    }

    session := newExecSession(ws)
    return session, nil
}
```

### 7.4 SDK Usage Examples

```go
// Interactive shell login
session, err := client.Sandbox.ExecInteractive(ctx, "abc123", &liteboxd.ExecInteractiveRequest{
    Command: []string{"/bin/bash"},
    TTY:     true,
    Cols:    120,
    Rows:    40,
})
if err != nil {
    log.Fatal(err)
}
defer session.Close()

// Write to stdin
session.Write([]byte("echo hello\n"))

// Read stdout
buf := make([]byte, 4096)
n, _ := session.Read(buf)
fmt.Print(string(buf[:n]))

// Resize terminal
session.Resize(200, 50)

// Wait for process to exit
exitCode := session.Wait()
```

---

## 8. Security Considerations

### 8.1 WebSocket Origin Check

- WebSocket upgrader 使用 `CheckOrigin: func(r *http.Request) bool { return true }` 是因为当前 API Server 无 auth.
- 如果将来加 auth, 应检查 Origin header 并要求 token 认证.

### 8.2 Exec Command Safety

- 用户可以 exec 任何命令, 这与现有的 `POST /exec` 一致.
- 容器本身已有安全隔离 (非 root, 网络策略, seccomp profile).
- 不增加额外的命令限制, 保持与 kubectl exec 一致的行为.

### 8.3 Session Timeout

- WebSocket 连接应有 idle timeout. 如果一段时间内没有 I/O, 自动断开.
- 建议: 30 分钟 idle timeout. 在 handler 中通过 `ws.SetReadDeadline()` 实现, 每收到消息重置.
- 容器的 TTL 不受终端连接影响.

### 8.4 Concurrent Sessions

- 同一个容器允许多个并发终端会话 (与 kubectl exec 行为一致).
- 每个 WebSocket 连接对应一个独立的 exec session.

---

## 9. Dependencies

### Backend (Go)

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/gorilla/websocket` | v1.5+ | WebSocket server |

### Web Frontend (npm)

| Package | Version | Purpose |
|---------|---------|---------|
| `@xterm/xterm` | ^5.x | Terminal emulator |
| `@xterm/addon-fit` | ^0.10.x | Auto-fit terminal to container |
| `@xterm/addon-web-links` | ^0.11.x | Clickable links in terminal |

### CLI (Go)

| Package | Version | Purpose |
|---------|---------|---------|
| `golang.org/x/term` | latest | Raw terminal mode, size detection |
| `github.com/gorilla/websocket` | v1.5+ | WebSocket client |

---

## 10. Implementation Plan

### Phase 1: Backend Core

1. Add `gorilla/websocket` dependency
2. Implement `k8s.ExecInteractive()` in `backend/internal/k8s/client.go`
3. Implement `k8s.SizeQueue` in `backend/internal/k8s/terminal.go`
4. Add `WSMessage` type to `backend/internal/model/sandbox.go`
5. Implement `SandboxService.ExecInteractive()` in `backend/internal/service/sandbox.go`
6. Implement WebSocket handler in `backend/internal/handler/sandbox.go`
7. Register new route in `backend/internal/handler/sandbox.go`
8. Add CORS header `Upgrade` to server config

### Phase 2: Web UI

1. Install xterm.js dependencies
2. Create `web/src/components/TerminalDialog.vue`
3. Update `web/src/views/SandboxDetail.vue` with "登录沙箱" button
4. Add WebSocket URL builder to `web/src/api/sandbox.ts`
5. Test in browser

### Phase 3: SDK

1. Add `ExecInteractiveRequest` and `WSMessage` to `backend/pkg/model/sandbox.go`
2. Add type aliases to `sdk/go/types.go`
3. Implement `ExecSession` in `sdk/go/exec_session.go`
4. Implement `SandboxService.ExecInteractive()` in `sdk/go/sandbox.go`
5. Add `gorilla/websocket` dependency to SDK

### Phase 4: CLI

1. Add `-i` and `-t` flags to `exec` command
2. Implement `runInteractiveExec()` with raw terminal mode
3. Implement SIGWINCH signal handling for terminal resize
4. Add `golang.org/x/term` and `gorilla/websocket` dependencies
5. Test with real container

### Phase 5: Testing & Polish

1. Unit tests for WebSocket protocol handling
2. Integration test: WebSocket exec with a real container
3. Test terminal resize behavior
4. Test connection cleanup on container exit / TTL expiry
5. Test concurrent sessions
6. Error handling edge cases (container not running, network disconnect)
