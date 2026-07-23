import { describe, it, expect } from 'vitest'
import { applyStepAdvanced, progressLabel, stepDisplayState } from './stepProgress'
import type { StepAdvancedEvent, StepCursorProgress, StepProgressStep } from '@/types/stepwise'

function makeStep(overrides: Partial<StepProgressStep> = {}): StepProgressStep {
  return {
    step_id: 'step-1',
    title: 'Do the thing',
    status: 'pending',
    ...overrides,
  }
}

function makeCursor(overrides: Partial<StepCursorProgress> = {}): StepCursorProgress {
  return {
    node_id: 'implementation',
    revision: 1,
    current_index: 2,
    total: 5,
    done: false,
    updated_at: '2026-01-01T00:00:00Z',
    steps: [],
    ...overrides,
  }
}

function makeEvent(overrides: Partial<StepAdvancedEvent> = {}): StepAdvancedEvent {
  return {
    workflow_instance_id: 'wi1',
    node_id: 'implementation',
    step_id: 'step-3',
    step_index: 3,
    total: 5,
    rejected_count: 0,
    rotated: false,
    ...overrides,
  }
}

describe('stepDisplayState', () => {
  it('maps pending/active/done statuses directly', () => {
    expect(stepDisplayState(makeStep({ status: 'pending' }))).toBe('pending')
    expect(stepDisplayState(makeStep({ status: 'active' }))).toBe('active')
    expect(stepDisplayState(makeStep({ status: 'done' }))).toBe('done')
  })

  it('maps rejected_retrying to rejected-retrying (rejections > 0 on the active step)', () => {
    expect(stepDisplayState(makeStep({ status: 'rejected_retrying', rejections: 2 }))).toBe(
      'rejected-retrying'
    )
  })

  it('rotated wins over done for display', () => {
    expect(stepDisplayState(makeStep({ status: 'done', rotated: true }))).toBe('rotated')
  })

  it('rotated wins over any other status too', () => {
    expect(stepDisplayState(makeStep({ status: 'active', rotated: true }))).toBe('rotated')
  })
})

describe('progressLabel', () => {
  it('shows N/M mid-run', () => {
    expect(progressLabel(makeCursor({ current_index: 1, total: 5 }))).toBe('2/5')
  })

  it('shows M/M when done (current_index === total)', () => {
    expect(progressLabel(makeCursor({ current_index: 5, total: 5 }))).toBe('5/5')
  })

  it('clamps current_index + 1 at total', () => {
    // current_index === total (all steps completed) should not overshoot to 6/5
    expect(progressLabel(makeCursor({ current_index: 5, total: 5 }))).toBe('5/5')
  })
})

describe('applyStepAdvanced', () => {
  it('advance moves current_index/total/current_step_id', () => {
    const prev = makeCursor({ current_index: 2, total: 5, current_step_id: 'step-2', done: false })
    const next = applyStepAdvanced(prev, makeEvent({ step_index: 3, step_id: 'step-3', total: 5 }))
    expect(next).toMatchObject({ current_index: 3, current_step_id: 'step-3', done: false })
  })

  it('marks done when step_index reaches total (step_id empty)', () => {
    const prev = makeCursor({ current_index: 4, total: 5 })
    const next = applyStepAdvanced(prev, makeEvent({ step_index: 5, step_id: '', total: 5 }))
    expect(next).toMatchObject({ current_index: 5, current_step_id: undefined, done: true })
  })

  it('a reject event (same step_index, rejected_count > 0) does not move the cursor', () => {
    const prev = makeCursor({ current_index: 2, total: 5, current_step_id: 'step-3' })
    const next = applyStepAdvanced(
      prev,
      makeEvent({ step_index: 2, step_id: 'step-3', rejected_count: 1 })
    )
    // The cursor's own current_index is unchanged; per-step rejection counts
    // live in the REST snapshot's steps[], refreshed via the invalidation
    // that useStepCursors triggers alongside this patch.
    expect(next?.current_index).toBe(2)
    expect(next?.current_step_id).toBe('step-3')
  })

  it('leaves cursor untouched when the event node_id does not match', () => {
    const prev = makeCursor({ node_id: 'implementation', current_index: 2 })
    const next = applyStepAdvanced(prev, makeEvent({ node_id: 'qa-verifier' }))
    expect(next).toBe(prev)
  })

  it('returns undefined (no-op) when there is no prior cursor', () => {
    expect(applyStepAdvanced(undefined, makeEvent())).toBeUndefined()
  })
})
