import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefStepwiseSection } from './AgentDefStepwiseSection'
import type { StepDefinition } from '@/types/workflow'

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) => (
    <textarea value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
  ),
}))

function makeStep(overrides: Partial<StepDefinition> = {}): StepDefinition {
  return { step_id: 'step-a', title: 'Step A', instruction: 'Do the thing', ...overrides }
}

describe('AgentDefStepwiseSection', () => {
  it('appends a step on "Add step"', async () => {
    const user = userEvent.setup()
    const onStepsChange = vi.fn()
    render(
      <AgentDefStepwiseSection promptMode="stepwise" onPromptModeChange={vi.fn()} steps={[makeStep()]} onStepsChange={onStepsChange} />
    )

    await user.click(screen.getByText('Add step'))

    expect(onStepsChange).toHaveBeenCalledWith([makeStep(), { step_id: '', title: '', instruction: '' }])
  })

  it('removes a step', async () => {
    const user = userEvent.setup()
    const onStepsChange = vi.fn()
    const steps = [makeStep({ step_id: 'a' }), makeStep({ step_id: 'b' })]
    render(
      <AgentDefStepwiseSection promptMode="stepwise" onPromptModeChange={vi.fn()} steps={steps} onStepsChange={onStepsChange} />
    )

    await user.click(screen.getAllByText('Remove')[0]!)

    expect(onStepsChange).toHaveBeenCalledWith([steps[1]])
  })

  it('moves a step down, swapping it with the next entry', async () => {
    const user = userEvent.setup()
    const onStepsChange = vi.fn()
    const steps = [makeStep({ step_id: 'a' }), makeStep({ step_id: 'b' }), makeStep({ step_id: 'c' })]
    render(
      <AgentDefStepwiseSection promptMode="stepwise" onPromptModeChange={vi.fn()} steps={steps} onStepsChange={onStepsChange} />
    )

    await user.click(screen.getAllByText('Down')[0]!)
    expect(onStepsChange).toHaveBeenCalledWith([steps[1], steps[0], steps[2]])
  })

  it('moves a step up, swapping it with the previous entry', async () => {
    const user = userEvent.setup()
    const onStepsChange = vi.fn()
    const steps = [makeStep({ step_id: 'a' }), makeStep({ step_id: 'b' }), makeStep({ step_id: 'c' })]
    render(
      <AgentDefStepwiseSection promptMode="stepwise" onPromptModeChange={vi.fn()} steps={steps} onStepsChange={onStepsChange} />
    )

    await user.click(screen.getAllByText('Up')[1]!)
    expect(onStepsChange).toHaveBeenCalledWith([steps[1], steps[0], steps[2]])
  })

  it('disables Up on the first step and Down on the last step', () => {
    const steps = [makeStep({ step_id: 'a' }), makeStep({ step_id: 'b' })]
    render(
      <AgentDefStepwiseSection promptMode="stepwise" onPromptModeChange={vi.fn()} steps={steps} onStepsChange={vi.fn()} />
    )

    const ups = screen.getAllByText('Up')
    const downs = screen.getAllByText('Down')
    expect(ups[0]).toBeDisabled()
    expect(downs[downs.length - 1]).toBeDisabled()
  })

  it('disables Add step at the 20-step cap', () => {
    const steps = Array.from({ length: 20 }, (_, i) => makeStep({ step_id: `step-${i}` }))
    render(
      <AgentDefStepwiseSection promptMode="stepwise" onPromptModeChange={vi.fn()} steps={steps} onStepsChange={vi.fn()} />
    )

    expect(screen.getByText('Add step')).toBeDisabled()
  })

  it('hides the step list when prompt mode is full', () => {
    render(
      <AgentDefStepwiseSection promptMode="full" onPromptModeChange={vi.fn()} steps={[makeStep()]} onStepsChange={vi.fn()} />
    )

    expect(screen.queryByText('Add step')).not.toBeInTheDocument()
  })
})
