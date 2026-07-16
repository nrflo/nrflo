import { Dropdown } from '@/components/ui/Dropdown'

export const CODEX_SANDBOX_OPTIONS = [
  { value: '', label: 'Full access (default)' },
  { value: 'read-only', label: 'Read-only (no file writes)' },
  { value: 'workspace-write', label: 'Workspace-write (writes inside workdir)' },
  { value: 'danger-full-access', label: 'Danger: full access (explicit)' },
]

/**
 * AgentDefSandboxField selects the codex app-server sandbox for an
 * openai-provider cli_interactive agent. Empty = danger-full-access (today's
 * autonomous default). Rendered only for codex CLI agents.
 */
export function AgentDefSandboxField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div>
      <label className="block text-xs font-medium text-muted-foreground mb-1">Sandbox (codex)</label>
      <Dropdown value={value} onChange={onChange} options={CODEX_SANDBOX_OPTIONS} />
      <p className="text-xs text-muted-foreground mt-1">
        Filesystem/network policy for the codex agent. Read-only blocks all writes; codex reports blocked
        actions instead of performing them.
      </p>
    </div>
  )
}
