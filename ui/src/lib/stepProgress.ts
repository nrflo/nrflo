// Pure, DOM-free display helpers for stepwise progress (kept out of the
// component tree so they load in the fast node vitest project).
import type { StepAdvancedEvent, StepCursorProgress, StepProgressStep } from '@/types/stepwise'

export type StepDisplayState = 'pending' | 'active' | 'done' | 'rejected-retrying' | 'rotated'

// rotated is orthogonal to status on the wire; it wins over 'done' for display.
export function stepDisplayState(step: StepProgressStep): StepDisplayState {
  if (step.rotated) return 'rotated'
  switch (step.status) {
    case 'active':
      return 'active'
    case 'done':
      return 'done'
    case 'rejected_retrying':
      return 'rejected-retrying'
    default:
      return 'pending'
  }
}

// step_index is the 0-based current_index (== completed count); displayed
// N is clamped to total so "all done" (step_index === total) reads N/M.
export function progressLabel(cursor: StepCursorProgress): string {
  const n = Math.min(cursor.current_index + 1, cursor.total)
  return `${n}/${cursor.total}`
}

export function applyStepAdvanced(
  prev: StepCursorProgress | undefined,
  ev: StepAdvancedEvent
): StepCursorProgress | undefined {
  if (!prev || prev.node_id !== ev.node_id) return prev
  return {
    ...prev,
    current_index: ev.step_index,
    total: ev.total,
    current_step_id: ev.step_id || undefined,
    done: ev.step_index >= ev.total,
    updated_at: new Date().toISOString(),
  }
}
