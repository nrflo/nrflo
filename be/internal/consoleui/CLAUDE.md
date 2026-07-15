# consoleui Package

Native Bubble Tea v2 client for a live `console_chat`; the server remains the sole owner of provider processes, conversation state, tools, and approvals. `client.go` carries the scoped bearer over REST and the authenticated WebSocket session subscription, while the model renders persisted history plus live deltas and reconnects the event stream. Enter submits, Shift+Enter/Alt+Enter/Ctrl+J inserts a newline, `y`/`n` resolves a pending approval, Ctrl+C interrupts a running turn, and Ctrl+D exits and lets the CLI close the chat.
