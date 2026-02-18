<template>
  <div class="terminal-page">
    <div class="terminal-header">
      <span class="terminal-title">Sandbox: {{ sandboxId }}</span>
      <div class="terminal-status">
        <t-tag v-if="connectionStatus === 'connecting'" theme="warning">连接中...</t-tag>
        <t-tag v-else-if="connectionStatus === 'connected'" theme="success">已连接</t-tag>
        <t-tag v-else-if="connectionStatus === 'disconnected'" theme="default">已断开</t-tag>
        <t-tag v-else-if="connectionStatus === 'error'" theme="danger">连接错误</t-tag>
      </div>
      <div class="terminal-actions">
        <t-button
          v-if="connectionStatus === 'disconnected' || connectionStatus === 'error'"
          theme="primary"
          size="small"
          @click="reconnect"
        >
          重新连接
        </t-button>
      </div>
    </div>
    <div ref="terminalRef" class="terminal-container"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

const route = useRoute()
const sandboxId = route.params.id as string

const terminalRef = ref<HTMLElement>()
const connectionStatus = ref<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting')

let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null

function isCopyShortcut(event: KeyboardEvent): boolean {
  const key = event.key.toLowerCase()
  return (event.ctrlKey || event.metaKey) && event.shiftKey && key === 'c'
}

async function copyTerminalSelection(event: KeyboardEvent): Promise<void> {
  const selection = terminal?.getSelection()
  if (!selection) return

  event.preventDefault()
  event.stopPropagation()

  try {
    await navigator.clipboard.writeText(selection)
  } catch {
    // Clipboard API may be blocked by browser policy; fallback to execCommand.
    const textarea = document.createElement('textarea')
    textarea.value = selection
    textarea.setAttribute('readonly', 'true')
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
  }
}

// Update window title
document.title = `Terminal - ${sandboxId}`

function connect() {
  if (!terminalRef.value) return

  connectionStatus.value = 'connecting'

  // Create terminal
  terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#d4d4d4',
      cursor: '#ffffff',
      cursorAccent: '#000000',
      black: '#000000',
      red: '#cd3131',
      green: '#0dbc79',
      yellow: '#e5e510',
      blue: '#2472c8',
      magenta: '#bc3fbc',
      cyan: '#11a8cd',
      white: '#e5e5e5',
      brightBlack: '#666666',
      brightRed: '#f14c4c',
      brightGreen: '#23d18b',
      brightYellow: '#f5f543',
      brightBlue: '#3b8eea',
      brightMagenta: '#d670d6',
      brightCyan: '#29b8db',
      brightWhite: '#e5e5e5',
    },
  })

  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(new WebLinksAddon())

  terminal.open(terminalRef.value)
  terminal.attachCustomKeyEventHandler((event: KeyboardEvent) => {
    if (event.type !== 'keydown') return true
    if (isCopyShortcut(event)) {
      void copyTerminalSelection(event)
      return false
    }
    return true
  })

  // Fit after a small delay to ensure the container is properly sized
  setTimeout(() => {
    fitAddon?.fit()
    connectWebSocket()
  }, 100)

  // Set up resize observer
  resizeObserver = new ResizeObserver(() => {
    fitAddon?.fit()
  })
  resizeObserver.observe(terminalRef.value)

  // Also handle window resize
  window.addEventListener('resize', handleWindowResize)
}

function handleWindowResize() {
  fitAddon?.fit()
}

function connectWebSocket() {
  if (!terminal || !fitAddon) return

  const { cols, rows } = terminal

  // Build WebSocket URL
  const baseUrl = import.meta.env.VITE_API_URL || '/api/v1'
  const cleanBase = baseUrl.replace(/^https?:\/\//, '').replace(/^\//, '')
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'

  let wsUrl: string
  if (baseUrl.startsWith('/')) {
    wsUrl = `${protocol}//${location.host}${baseUrl}/sandboxes/${sandboxId}/exec/interactive?command=sh&tty=true&cols=${cols}&rows=${rows}`
  } else {
    wsUrl = `${protocol}//${cleanBase}/sandboxes/${sandboxId}/exec/interactive?command=sh&tty=true&cols=${cols}&rows=${rows}`
  }

  ws = new WebSocket(wsUrl)

  ws.onopen = () => {
    connectionStatus.value = 'connected'
    terminal?.focus()
  }

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data)
      switch (msg.type) {
        case 'output':
          terminal?.write(msg.data)
          break
        case 'exit':
          terminal?.writeln(`\r\n\x1b[33m[Process exited with code ${msg.exitCode}]\x1b[0m`)
          connectionStatus.value = 'disconnected'
          break
        case 'error':
          terminal?.writeln(`\r\n\x1b[31m[Error: ${msg.message}]\x1b[0m`)
          connectionStatus.value = 'error'
          break
      }
    } catch {
      terminal?.write(event.data)
    }
  }

  ws.onerror = () => {
    connectionStatus.value = 'error'
  }

  ws.onclose = () => {
    if (connectionStatus.value === 'connected') {
      terminal?.writeln('\r\n\x1b[33m[Connection closed]\x1b[0m')
      connectionStatus.value = 'disconnected'
    }
  }

  // Forward terminal input to WebSocket
  terminal.onData((data) => {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'input', data }))
    }
  })

  // Handle terminal resize
  terminal.onResize(({ cols, rows }) => {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'resize', cols, rows }))
    }
  })
}

function reconnect() {
  if (ws) {
    ws.close()
    ws = null
  }
  terminal?.clear()
  connectWebSocket()
}

function disconnect() {
  window.removeEventListener('resize', handleWindowResize)
  resizeObserver?.disconnect()
  resizeObserver = null
  ws?.close()
  ws = null
  terminal?.dispose()
  terminal = null
  fitAddon = null
}

onMounted(() => {
  connect()
})

onUnmounted(() => {
  disconnect()
})
</script>

<style scoped>
.terminal-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #1e1e1e;
  overflow: hidden;
}

.terminal-header {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 8px 16px;
  background: #2d2d2d;
  border-bottom: 1px solid #3d3d3d;
  flex-shrink: 0;
}

.terminal-title {
  color: #d4d4d4;
  font-family: monospace;
  font-size: 14px;
  font-weight: 500;
}

.terminal-status {
  flex-shrink: 0;
}

.terminal-actions {
  margin-left: auto;
}

.terminal-container {
  flex: 1;
  padding: 4px;
  overflow: hidden;
}
</style>
