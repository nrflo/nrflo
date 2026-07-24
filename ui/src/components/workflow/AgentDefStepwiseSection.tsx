import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { StepDefinitionEditor } from './StepDefinitionEditor'
import { validateStepDefinitions } from '@/lib/stepDefinitions'
import type { PromptMode, StepDefinition } from '@/types/workflow'

const PROMPT_MODE_OPTIONS = [
  { value: 'full', label: 'Full (single prompt)' },
  { value: 'stepwise', label: 'Stepwise (ordered steps)' },
]

const emptyStep = (): StepDefinition => ({ step_id: '', title: '', instruction: '' })

// Container rendered by AgentDefForm for non-script modes: prompt_mode
// toggle + ordered step list (add/remove/reorder) + inline validation errors.
export function AgentDefStepwiseSection({
  promptMode,
  onPromptModeChange,
  steps,
  onStepsChange,
}: {
  promptMode: PromptMode
  onPromptModeChange: (mode: PromptMode) => void
  steps: StepDefinition[]
  onStepsChange: (next: StepDefinition[]) => void
}) {
  const errors = promptMode === 'stepwise' ? validateStepDefinitions(steps) : []

  const move = (idx: number, dir: -1 | 1) => {
    const next = [...steps]
    const swapIdx = idx + dir
    if (swapIdx < 0 || swapIdx >= next.length) return
    ;[next[idx], next[swapIdx]] = [next[swapIdx]!, next[idx]!]
    onStepsChange(next)
  }

  return (
    <div className="space-y-2">
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Prompt mode</label>
        <Dropdown value={promptMode} onChange={(v) => onPromptModeChange(v as PromptMode)} options={PROMPT_MODE_OPTIONS} />
      </div>
      {promptMode === 'stepwise' && (
        <div className="space-y-2">
          {steps.map((step, idx) => (
            <div key={idx} className="space-y-1">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">Step {idx + 1}</span>
                <div className="flex items-center gap-1">
                  <Button type="button" variant="ghost" size="sm" disabled={idx === 0} onClick={() => move(idx, -1)}>Up</Button>
                  <Button type="button" variant="ghost" size="sm" disabled={idx === steps.length - 1} onClick={() => move(idx, 1)}>Down</Button>
                  <Button type="button" variant="ghost" size="sm" onClick={() => onStepsChange(steps.filter((_, i) => i !== idx))}>Remove</Button>
                </div>
              </div>
              <StepDefinitionEditor step={step} onChange={(next) => onStepsChange(steps.map((s, i) => (i === idx ? next : s)))} />
            </div>
          ))}
          <Button type="button" variant="outline" size="sm" disabled={steps.length >= 20} onClick={() => onStepsChange([...steps, emptyStep()])}>
            Add step
          </Button>
          {errors.length > 0 && (
            <div className="rounded-md border border-destructive/30 bg-destructive/5 p-2 space-y-0.5">
              {errors.map((e, i) => (
                <p key={i} className="text-xs text-destructive">{e}</p>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
