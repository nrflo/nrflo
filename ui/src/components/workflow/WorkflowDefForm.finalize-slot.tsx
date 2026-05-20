import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { PythonScriptPickerField } from '@/components/workflow/PythonScriptPickerField'

export type FinalizeSlotMode = 'command' | 'script' | ''

interface FinalizeSlotProps {
  label: string
  mode: FinalizeSlotMode
  command: string
  scriptId: string
  onModeChange: (m: FinalizeSlotMode) => void
  onCommandChange: (v: string) => void
  onScriptIdChange: (v: string) => void
}

function FinalizeSlot({ label, mode, command, scriptId, onModeChange, onCommandChange, onScriptIdChange }: FinalizeSlotProps) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground w-20">{label}</span>
        <Button
          type="button"
          variant={mode === 'command' ? 'default' : 'outline'}
          size="sm"
          onClick={() => onModeChange(mode === 'command' ? '' : 'command')}
        >
          Command
        </Button>
        <Button
          type="button"
          variant={mode === 'script' ? 'default' : 'outline'}
          size="sm"
          onClick={() => onModeChange(mode === 'script' ? '' : 'script')}
        >
          Script
        </Button>
      </div>
      {mode === 'command' && (
        <Input
          type="text"
          value={command}
          onChange={(e) => onCommandChange(e.target.value)}
          placeholder="Shell command to run"
        />
      )}
      {mode === 'script' && (
        <PythonScriptPickerField value={scriptId} onChange={onScriptIdChange} />
      )}
    </div>
  )
}

export interface FinalizeState {
  successMode: FinalizeSlotMode
  successCommand: string
  successScriptId: string
  failureMode: FinalizeSlotMode
  failureCommand: string
  failureScriptId: string
}

interface FinalizeSectionProps {
  state: FinalizeState
  onChange: (patch: Partial<FinalizeState>) => void
}

export function FinalizeSection({ state, onChange }: FinalizeSectionProps) {
  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-xs font-medium text-muted-foreground">Finalize</div>
      <FinalizeSlot
        label="On success"
        mode={state.successMode}
        command={state.successCommand}
        scriptId={state.successScriptId}
        onModeChange={(m) => onChange({ successMode: m, successCommand: '', successScriptId: '' })}
        onCommandChange={(v) => onChange({ successCommand: v })}
        onScriptIdChange={(v) => onChange({ successScriptId: v })}
      />
      <FinalizeSlot
        label="On failure"
        mode={state.failureMode}
        command={state.failureCommand}
        scriptId={state.failureScriptId}
        onModeChange={(m) => onChange({ failureMode: m, failureCommand: '', failureScriptId: '' })}
        onCommandChange={(v) => onChange({ failureCommand: v })}
        onScriptIdChange={(v) => onChange({ failureScriptId: v })}
      />
    </div>
  )
}
