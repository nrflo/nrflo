import { useRef } from 'react'
import { MarkdownEditor, type MarkdownEditorHandle } from '@/components/ui/MarkdownEditor'
import { renderTemplatePreview } from '@/utils/renderTemplatePreview'
import type { ChannelKind } from '@/types/notifications'

// Sample payload used for live preview — mirrors backend notify.buildVars keys.
const SAMPLE: Record<string, string> = {
  event_type: 'orchestration.completed',
  link: 'http://localhost:6587/tickets/PROJ-123',
  summary: '',
  project_name: 'My Project',
  project_id: 'proj-abc',
  ticket_name: 'Add user authentication',
  ticket_id: 'PROJ-123',
  workflow: 'feature',
  instance_id: 'abc-def-123',
  agent_type: 'implementor',
  reason: '',
}

export function NotificationTemplateEditor({
  kind,
  value,
  onChange,
  variables,
}: {
  kind: ChannelKind
  value: string
  onChange: (v: string) => void
  variables: string[]
}) {
  const editorRef = useRef<MarkdownEditorHandle>(null)
  const preview = renderTemplatePreview(value, SAMPLE)

  const handleChipClick = (varName: string) => {
    editorRef.current?.insertAtCaret(`\${${varName}}`)
  }

  return (
    <div className="space-y-2">
      <label className="text-sm font-medium text-muted-foreground block">
        Message Template
      </label>

      <MarkdownEditor
        ref={editorRef}
        value={value}
        onChange={onChange}
        placeholder={`Message template for ${kind} notifications…`}
        minHeight="120px"
        maxHeight="240px"
      />

      {variables.length > 0 && (
        <div className="space-y-1">
          <div className="text-xs text-muted-foreground">Click to insert variable:</div>
          <div className="flex flex-wrap gap-1.5">
            {variables.map((v) => (
              <button
                key={v}
                type="button"
                onClick={() => handleChipClick(v)}
                className="text-xs font-mono px-2 py-0.5 rounded border border-primary/40 text-primary bg-primary/5 hover:bg-primary/15 transition-colors"
              >
                {`\${${v}}`}
              </button>
            ))}
          </div>
        </div>
      )}

      <p className="text-xs text-muted-foreground italic">
        Preview is structure-only — Telegram MarkdownV2 escaping is applied server-side.
      </p>

      {value.trim() && (
        <div className="space-y-1">
          <div className="text-xs text-muted-foreground">Preview:</div>
          <pre className="text-xs bg-muted/50 border rounded p-2 whitespace-pre-wrap break-words font-mono leading-relaxed">
            {preview || <span className="text-muted-foreground italic">empty</span>}
          </pre>
        </div>
      )}
    </div>
  )
}
