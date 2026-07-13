import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PlanReviseDialog } from './PlanReviseDialog'
import type { PlanTemplate } from '@/types/plan'

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

vi.mock('@/hooks/usePlan', () => ({
  useRevisePlan: vi.fn(),
}))

import { useRevisePlan } from '@/hooks/usePlan'

function makeTemplate(overrides: Partial<PlanTemplate> = {}): PlanTemplate {
  return {
    id: 'setup-analyzer',
    model: 'sonnet',
    execution_mode: 'cli_interactive',
    prompt: 'Investigate the codebase',
    description: 'Runs first to scope the change',
    ...overrides,
  }
}

describe('PlanReviseDialog — templates catalog', () => {
  const onClose = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useRevisePlan).mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
  })

  it('renders no templates section when templates is undefined', () => {
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} />)
    expect(screen.queryByText('Available templates')).not.toBeInTheDocument()
  })

  it('renders no templates section when templates is empty', () => {
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} templates={[]} />)
    expect(screen.queryByText('Available templates')).not.toBeInTheDocument()
  })

  it('lists each template with id, model, and description', () => {
    const templates = [
      makeTemplate({ id: 'setup-analyzer', model: 'sonnet', description: 'Runs first to scope the change' }),
      makeTemplate({ id: 'implementor', model: 'opus', description: 'Writes the code' }),
    ]
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} templates={templates} />)

    expect(screen.getByText('Available templates')).toBeInTheDocument()
    expect(screen.getByText('setup-analyzer')).toBeInTheDocument()
    expect(screen.getByText('sonnet')).toBeInTheDocument()
    expect(screen.getByText('Runs first to scope the change')).toBeInTheDocument()
    expect(screen.getByText('implementor')).toBeInTheDocument()
    expect(screen.getByText('opus')).toBeInTheDocument()
    expect(screen.getByText('Writes the code')).toBeInTheDocument()
  })

  it('omits the description line when a template has an empty description', () => {
    const templates = [makeTemplate({ id: 'bare-template', description: '' })]
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} templates={templates} />)

    expect(screen.getByText('bare-template')).toBeInTheDocument()
    expect(screen.queryByText('Runs first to scope the change')).not.toBeInTheDocument()
  })
})
