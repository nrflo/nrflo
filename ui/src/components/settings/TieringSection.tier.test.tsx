import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { TieringSection } from './TieringSection'
import { createTestQueryClient } from '@/test/utils'
import { useApplyTiering, useTieringReport } from '@/hooks/useTiering'
import type { TieringDefRow, TieringProjectReport, TieringReport } from '@/types/tiering'

vi.mock('@/hooks/useTiering')

function makeDef(overrides: Partial<TieringDefRow> = {}): TieringDefRow {
  return {
    workflow_id: 'feature',
    def_id: 'implementor',
    role: 'implementor',
    is_worker: true,
    current_tier: 2,
    current_model: 'sonnet-5',
    current_effort: 'high',
    recommended_tier: 1,
    recommended_model: 'sonnet-5',
    recommended_effort: 'medium',
    recommended_template: 'tier-t1-executor',
    grants_delegation: false,
    customized: false,
    est_monthly_delta: -12.5,
    ...overrides,
  }
}

function makeProject(overrides: Partial<TieringProjectReport> = {}): TieringProjectReport {
  return {
    project_id: 'proj-a',
    project_name: 'Project A',
    defs: [makeDef()],
    est_monthly_delta: -12.5,
    ...overrides,
  }
}

function makeReport(projects: TieringProjectReport[]): TieringReport {
  return { projects, markdown: '# Tiering Report' }
}

function renderSection() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <TieringSection />
    </QueryClientProvider>
  )
}

const idleApply = { mutate: vi.fn(), isPending: false, isError: false, error: null, variables: undefined }

describe('TieringSection - tier columns', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useApplyTiering).mockReturnValue(idleApply as any)
  })

  it('renders "— · model / effort" for a def with a null current_tier (untiered)', () => {
    const project = makeProject({
      defs: [makeDef({ current_tier: null, current_model: 'sonnet-5', current_effort: 'high' })],
    })
    vi.mocked(useTieringReport).mockReturnValue({ isLoading: false, error: null, data: makeReport([project]) } as any)
    renderSection()

    expect(screen.getByText(/— · sonnet-5 \/ high/)).toBeInTheDocument()
  })

  it('renders "Tier N · model / effort" for a def with a populated current_tier', () => {
    const project = makeProject({
      defs: [makeDef({ current_tier: 3, current_model: 'opus-4-8', current_effort: 'medium' })],
    })
    vi.mocked(useTieringReport).mockReturnValue({ isLoading: false, error: null, data: makeReport([project]) } as any)
    renderSection()

    expect(screen.getByText(/Tier 3 · opus-4-8 \/ medium/)).toBeInTheDocument()
  })

  it('still shows its skip/applicable badge when recommended_tier equals current_tier', () => {
    const project = makeProject({
      defs: [
        makeDef({ def_id: 'unchanged', current_tier: 2, recommended_tier: 2, skip_reason: 'customized' }),
      ],
    })
    vi.mocked(useTieringReport).mockReturnValue({ isLoading: false, error: null, data: makeReport([project]) } as any)
    renderSection()

    expect(screen.getByText(/Tier 2 · sonnet-5 \/ high/)).toBeInTheDocument()
    expect(screen.getByText(/Tier 2 · sonnet-5 \/ medium/)).toBeInTheDocument()
    expect(screen.getByText('Customized — skip')).toBeInTheDocument()
  })

  it('renders the recommended column tier + model/effort for the applicable case', () => {
    const project = makeProject({
      defs: [makeDef({ recommended_tier: 1, recommended_model: 'sonnet-5', recommended_effort: 'medium' })],
    })
    vi.mocked(useTieringReport).mockReturnValue({ isLoading: false, error: null, data: makeReport([project]) } as any)
    renderSection()

    expect(screen.getByText(/Tier 1 · sonnet-5 \/ medium/)).toBeInTheDocument()
    expect(screen.getByText('Applicable')).toBeInTheDocument()
  })
})
