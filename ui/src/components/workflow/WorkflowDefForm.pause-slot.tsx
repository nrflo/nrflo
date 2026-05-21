import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { PythonScriptPickerField } from '@/components/workflow/PythonScriptPickerField'

export type PauseSlotMode = 'command' | 'script' | ''

export interface PauseState {
  mode: PauseSlotMode
  command: string
  scriptId: string
}

interface PauseSectionProps {
  state: PauseState
  onChange: (patch: Partial<PauseState>) => void
}

export function PauseSection({ state, onChange }: PauseSectionProps) {
  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-xs font-medium text-muted-foreground">Pause event hook</div>
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground w-20">On pause</span>
          <Button
            type="button"
            variant={state.mode === 'command' ? 'default' : 'outline'}
            size="sm"
            onClick={() => onChange({ mode: state.mode === 'command' ? '' : 'command', command: '', scriptId: '' })}
          >
            Command
          </Button>
          <Button
            type="button"
            variant={state.mode === 'script' ? 'default' : 'outline'}
            size="sm"
            onClick={() => onChange({ mode: state.mode === 'script' ? '' : 'script', command: '', scriptId: '' })}
          >
            Script
          </Button>
        </div>
        {state.mode === 'command' && (
          <Input
            type="text"
            value={state.command}
            onChange={(e) => onChange({ command: e.target.value })}
            placeholder="Shell command to run when workflow pauses"
          />
        )}
        {state.mode === 'script' && (
          <PythonScriptPickerField value={state.scriptId} onChange={(v) => onChange({ scriptId: v })} />
        )}
      </div>
    </div>
  )
}
