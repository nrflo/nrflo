interface ChatThinkingProps {
  text: string
}

// Collapsed-by-default thinking block — same convention as MessageTable's
// isThinking branch ('text-muted-foreground italic', badge 'Thinking').
// Sources: persisted category='thinking' rows (claude, capture_thinking) and
// live console_chat.thinking pushes (codex — live-only, does not survive
// reload; not faked as persisted here).
export function ChatThinking({ text }: ChatThinkingProps) {
  if (!text) return null

  return (
    <details className="rounded-md border border-border bg-muted/30 px-3 py-2">
      <summary className="cursor-pointer select-none">
        <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold bg-muted text-muted-foreground border border-border">
          Thinking
        </span>
      </summary>
      <div className="mt-1.5 text-sm text-muted-foreground italic whitespace-pre-wrap break-words">{text}</div>
    </details>
  )
}
