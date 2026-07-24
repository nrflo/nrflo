import { Input } from '@/components/ui/Input'
import { Toggle } from '@/components/ui/Toggle'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'
import { StepRequiredFindingsEditor } from './StepRequiredFindingsEditor'
import { StepChecksOverlapEditor } from './StepChecksOverlapEditor'
import type { StepDefinition } from '@/types/workflow'

// Single-step editor: scalar fields + rotation_allowed + the two sub-editors.
export function StepDefinitionEditor({
  step,
  onChange,
}: {
  step: StepDefinition
  onChange: (next: StepDefinition) => void
}) {
  return (
    <div className="space-y-3 rounded-md border border-border bg-background p-3">
      <div className="flex gap-3">
        <div className="w-48">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Step ID</label>
          <Input value={step.step_id} onChange={(e) => onChange({ ...step, step_id: e.target.value })} placeholder="e.g., write-tests" />
        </div>
        <div className="flex-1">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Title</label>
          <Input value={step.title} onChange={(e) => onChange({ ...step, title: e.target.value })} placeholder="Short step title" />
        </div>
        <div className="flex items-end pb-1.5">
          <Toggle
            checked={!!step.rotation_allowed}
            onChange={(checked) => onChange({ ...step, rotation_allowed: checked })}
            label="Rotation allowed"
          />
        </div>
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Instruction</label>
        <MarkdownEditor
          value={step.instruction}
          onChange={(v) => onChange({ ...step, instruction: v })}
          placeholder="Step instruction (markdown)..."
          minHeight="120px"
          maxHeight="320px"
        />
      </div>
      <StepRequiredFindingsEditor
        value={step.required_findings ?? []}
        onChange={(required_findings) => onChange({ ...step, required_findings })}
      />
      <StepChecksOverlapEditor
        checks={step.checks ?? []}
        onChecksChange={(checks) => onChange({ ...step, checks })}
        pathOverlap={step.path_overlap}
        onPathOverlapChange={(path_overlap) => onChange({ ...step, path_overlap })}
      />
    </div>
  )
}
