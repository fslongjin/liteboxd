<template>
  <t-dialog
    v-model:visible="visible"
    header="登录沙箱"
    :footer="false"
    width="80%"
    placement="center"
    :close-on-overlay-click="false"
    @close="handleClose"
  >
    <div class="terminal-wrapper">
      <div ref="terminalRef" class="terminal-container"></div>
      <div v-if="connectionStatus !== 'connected'" class="terminal-overlay">
        <t-loading v-if="connectionStatus === 'connecting'" text="连接中..." />
        <div v-else-if="connectionStatus === 'disconnected'" class="disconnected-message">
          <p>连接已断开</p>
          <t-button theme="primary" size="small" @click="reconnect">重新连接</t-button>
        </div>
        <div v-else-if="connectionStatus === 'error'" class="error-message">
          <p>{{ errorMessage }}</p>
          <t-button theme="primary" size="small" @click="reconnect">重新连接</t-button>
        </div>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted, nextTick, computed } from 'vue'
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
const connectionStatus = ref<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting')
const errorMessage = ref('')

let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val),
})

function connect() {
  if (!terminalRef.value) return

  connectionStatus.value = 'connecting'
  errorMessage.value = ''

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
}

function connectWebSocket() {
  if (!terminal || !fitAddon) return

  const { cols, rows } = terminal

  // Build WebSocket URL
  const baseUrl = import.meta.env.VITE_API_URL || '/api/v1'
  // Remove protocol prefix if exists and rebuild
  const cleanBase = baseUrl.replace(/^https?:\/\//, '').replace(/^\//, '')
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'

  let wsUrl: string
  if (baseUrl.startsWith('/')) {
    // Relative path
    wsUrl = `${protocol}//${location.host}${baseUrl}/sandboxes/${props.sandboxId}/exec/interactive?command=sh&tty=true&cols=${cols}&rows=${rows}`
  } else {
    // Absolute URL
    wsUrl = `${protocol}//${cleanBase}/sandboxes/${props.sandboxId}/exec/interactive?command=sh&tty=true&cols=${cols}&rows=${rows}`
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
          errorMessage.value = msg.message
          connectionStatus.value = 'error'
          break
      }
    } catch {
      // If not JSON, treat as raw output
      terminal?.write(event.data)
    }
  }

  ws.onerror = () => {
    errorMessage.value = '连接失败'
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
  resizeObserver?.disconnect()
  resizeObserver = null
  ws?.close()
  ws = null
  terminal?.dispose()
  terminal = null
  fitAddon = null
}

function handleClose() {
  disconnect()
  emit('update:visible', false)
}

watch(
  () => props.visible,
  (val) => {
    if (val) {
      nextTick(() => connect())
    } else {
      disconnect()
    }
  }
)

onUnmounted(() => {
  disconnect()
})
</script>

<style scoped>
.terminal-wrapper {
  position: relative;
  height: 500px;
  background: #1e1e1e;
  border-radius: 4px;
  overflow: hidden;
}

.terminal-container {
  width: 100%;
  height: 100%;
  padding: 4px;
  box-sizing: border-box;
}

.terminal-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(30, 30, 30, 0.9);
  color: #d4d4d4;
}

.disconnected-message,
.error-message {
  text-align: center;
}

.disconnected-message p,
.error-message p {
  margin-bottom: 16px;
}

.error-message p {
  color: #f14c4c;
}
</style>
