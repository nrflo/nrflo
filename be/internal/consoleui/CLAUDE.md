# consoleui Package

Native Bubble Tea v2 client for server-owned `console_chat` engines. The server catalog drives the resume/new-session picker; the main model keeps a bounded, paginated, render-cached transcript and reconciles history, live deltas/thinking, approvals, and turn state on every WebSocket reconnect. Enter submits, `y`/`n` resolves approval (`a` allows the tool for the whole session), Ctrl+C interrupts, Ctrl+F searches, Ctrl+G enters viewport/copy mode, Ctrl+D detaches, and Ctrl+X closes.
