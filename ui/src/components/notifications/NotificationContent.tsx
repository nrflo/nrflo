import { RenderedMarkdown } from '@/components/ui/RenderedMarkdown'

export function NotificationContent({ value }: { value: unknown }) {
  if (value === null || value === undefined || value === '') return null
  let structured: unknown = typeof value === 'object' ? value : undefined
  if (typeof value === 'string') {
    try {
      const parsed: unknown = JSON.parse(value)
      if (parsed !== null && typeof parsed === 'object') structured = parsed
    } catch {
      // Prose is rendered as Markdown below.
    }
  }
  if (structured !== undefined) {
    return <pre className="max-w-full overflow-x-auto rounded-md bg-muted p-3 text-xs whitespace-pre-wrap break-words"><code>{JSON.stringify(structured, null, 2)}</code></pre>
  }
  return <div className="min-w-0 break-words text-sm [&_pre]:overflow-x-auto"><RenderedMarkdown content={String(value)} /></div>
}
