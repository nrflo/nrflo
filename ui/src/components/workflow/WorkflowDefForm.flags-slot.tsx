import { Toggle } from '@/components/ui/Toggle'

interface FlagTogglesProps {
  purgeOnCompletion: boolean
  onPurgeChange: (v: boolean) => void
  callableAsSubworkflow: boolean
  onCallableChange: (v: boolean) => void
  /** callable requires project scope; sub-workflows run without a ticket */
  callableAllowed: boolean
}

/** Purge-on-completion + callable-as-sub-workflow toggles (mutually exclusive pair). */
export function FlagTogglesSection({
  purgeOnCompletion,
  onPurgeChange,
  callableAsSubworkflow,
  onCallableChange,
  callableAllowed,
}: FlagTogglesProps) {
  return (
    <>
      <div>
        <Toggle
          checked={purgeOnCompletion}
          onChange={onPurgeChange}
          label="Purge sensitive data after the run completes"
          disabled={callableAsSubworkflow}
        />
        <p className="text-xs text-muted-foreground mt-1">
          On completion, scrubs agent prompts/messages, findings, artifacts, and caller inputs,
          keeping only a redacted execution record. The final result is delivered via events, not retained.
        </p>
      </div>

      <div>
        <Toggle
          checked={callableAsSubworkflow}
          onChange={onCallableChange}
          disabled={!callableAsSubworkflow && (purgeOnCompletion || !callableAllowed)}
          label="Callable as sub-workflow"
        />
        <p className="text-xs text-muted-foreground mt-1">
          Lets agents start this workflow with the run_subworkflow tool and read its result back.
          Requires project scope (sub-workflows run without a ticket) and purge off (the result
          finding must survive completion).
        </p>
      </div>
    </>
  )
}
