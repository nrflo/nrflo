import { useEffect, useRef, useCallback } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'

interface XTerminalProps {
  sessionId: string
  onExit: () => void
}

function getPtyWebSocketUrl(sessionId: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const apiUrl = import.meta.env.VITE_API_URL
  if (apiUrl) {
    const url = new URL(apiUrl)
    return `${protocol}//${url.host}/api/v1/pty/${encodeURIComponent(sessionId)}`
  }
  return `${protocol}//${window.location.host}/api/v1/pty/${encodeURIComponent(sessionId)}`
}

export function XTerminal({ sessionId, onExit }: XTerminalProps) {
  const termRef = useRef<HTMLDivElement>(null)
  const terminalRef = useRef<Terminal | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const resizeTimerRef = useRef<number | null>(null)
  const onExitRef = useRef(onExit)
  onExitRef.current = onExit

  const sendResize = useCallback((cols: number, rows: number) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'resize', rows, cols }))
    }
  }, [])

  useEffect(() => {
    if (!termRef.current) return

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1a1a2e',
        foreground: '#e0e0e0',
        cursor: '#f0f0f0',
      },
    })

    const fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)
    terminal.open(termRef.current)
    fitAddon.fit()

    terminalRef.current = terminal
    fitAddonRef.current = fitAddon

    // Connect to PTY WebSocket
    const url = getPtyWebSocketUrl(sessionId)
    const ws = new WebSocket(url)
    ws.binaryType = 'arraybuffer'
    wsRef.current = ws

    ws.onopen = () => {
      // Send initial resize
      sendResize(terminal.cols, terminal.rows)
    }

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        terminal.write(new Uint8Array(event.data))
      } else {
        terminal.write(event.data)
      }
    }

    ws.onclose = () => {
      terminal.write('\r\n\x1b[90m[Session ended]\x1b[0m\r\n')
      onExitRef.current()
    }

    ws.onerror = () => {
      terminal.write('\r\n\x1b[31m[Connection error]\x1b[0m\r\n')
    }

    // User keystrokes → WebSocket
    const dataDisposable = terminal.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })

    // Terminal resize → WebSocket (debounced)
    const resizeDisposable = terminal.onResize(({ cols, rows }) => {
      if (resizeTimerRef.current) {
        clearTimeout(resizeTimerRef.current)
      }
      resizeTimerRef.current = window.setTimeout(() => {
        sendResize(cols, rows)
      }, 150)
    })

    // Window resize → fit terminal
    const handleWindowResize = () => {
      fitAddon.fit()
    }
    window.addEventListener('resize', handleWindowResize)

    // ResizeObserver for container resize
    const observer = new ResizeObserver(() => {
      fitAddon.fit()
    })
    observer.observe(termRef.current)

    return () => {
      if (resizeTimerRef.current) clearTimeout(resizeTimerRef.current)
      dataDisposable.dispose()
      resizeDisposable.dispose()
      window.removeEventListener('resize', handleWindowResize)
      observer.disconnect()
      ws.close()
      terminal.dispose()
      terminalRef.current = null
      wsRef.current = null
      fitAddonRef.current = null
    }
  }, [sessionId, sendResize])

  return (
    <div
      ref={termRef}
      className="w-full h-full"
      style={{ minHeight: '300px' }}
    />
  )
}
