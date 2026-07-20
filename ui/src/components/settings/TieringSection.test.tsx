import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
    current_model: 'sonnet-5',
    current_effort: 'high',
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

describe('TieringSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useApplyTiering).mockReturnValue(idleApply as any)
  })

  it('shows loading spinner while fetching', () => {
    vi.mocked(useTieringReport).mockReturnValue({ isLoading: true, data: undefined, error: null } as any)
    renderSection()
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('shows empty state when no projects', () => {
    vi.mocked(useTieringReport).mockReturnValue({
      isLoading: false,
      error: null,
      data: makeReport([]),
    } as any)
    renderSection()
    expect(screen.getByText('No projects to report on.')).toBeInTheDocument()
  })

  it('shows an error message when the report fails to load', () => {
    vi.mocked(useTieringReport).mockReturnValue({
      isLoading: false,
      error: new Error('boom'),
      data: undefined,
    } as any)
    renderSection()
    expect(screen.getByText('boom')).toBeInTheDocument()
  })

  it('renders per-project defs with current/recommended values and status flags', () => {
    const stockProject = makeProject({
      project_id: 'proj-a',
      project_name: 'Project A',
      defs: [makeDef({ def_id: 'implementor', current_model: 'fable-5', recommended_model: 'sonnet-5' })],
      est_monthly_delta: -12.5,
    })
    const customizedProject = makeProject({
      project_id: 'proj-b',
      project_name: 'Project B',
      defs: [
        makeDef({
          def_id: 'implementor-custom',
          current_model: 'fable-5',
          recommended_model: 'sonnet-5',
          customized: true,
          skip_reason: 'customized',
          est_monthly_delta: null,
        }),
      ],
      est_monthly_delta: null,
    })

    vi.mocked(useTieringReport).mockReturnValue({
      isLoading: false,
      error: null,
      data: makeReport([stockProject, customizedProject]),
    } as any)
    renderSection()

    expect(screen.getByText('Project A')).toBeInTheDocument()
    expect(screen.getByText('Project B')).toBeInTheDocument()
    expect(screen.getAllByText(/fable-5 \/ high/).length).toBe(2)
    expect(screen.getAllByText(/sonnet-5 \/ medium/).length).toBe(2)
    expect(screen.getAllByText('$-12.50/mo').length).toBeGreaterThan(0)

    // customized def is flagged and excluded from the applicable count
    expect(screen.getByText('Customized — skip')).toBeInTheDocument()
    expect(screen.getByText('Applicable')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Apply (1)' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Apply (0)' })).toBeInTheDocument()
  })

  it('shows a +delegation marker only for defs that grant delegation tools', () => {
    const project = makeProject({
      project_id: 'proj-a',
      defs: [
        makeDef({ def_id: 'implementor', grants_delegation: true }),
        makeDef({ def_id: 'qa-verifier', role: 'qa-verifier', grants_delegation: false }),
      ],
    })
    vi.mocked(useTieringReport).mockReturnValue({
      isLoading: false,
      error: null,
      data: makeReport([project]),
    } as any)
    renderSection()

    expect(screen.getAllByText(/\+delegation/).length).toBe(1)
  })

  it('renders a dash for defs and projects with a null estimated delta', () => {
    const project = makeProject({
      defs: [makeDef({ est_monthly_delta: null })],
      est_monthly_delta: null,
    })
    vi.mocked(useTieringReport).mockReturnValue({
      isLoading: false,
      error: null,
      data: makeReport([project]),
    } as any)
    renderSection()
    expect(screen.getAllByText('—').length).toBeGreaterThan(0)
  })

  it('clicking Apply calls the mutation once with that project confirmation payload', async () => {
    const user = userEvent.setup()
    const project = makeProject({ project_id: 'proj-a', defs: [makeDef()] })
    vi.mocked(useTieringReport).mockReturnValue({
      isLoading: false,
      error: null,
      data: makeReport([project]),
    } as any)
    renderSection()

    await user.click(screen.getByRole('button', { name: 'Apply (1)' }))

    expect(idleApply.mutate).toHaveBeenCalledTimes(1)
    expect(idleApply.mutate).toHaveBeenCalledWith({
      confirmations: [{ project_id: 'proj-a', confirm_all: true }],
    })
  })

  it('disables the Apply button while a mutation is pending', () => {
    vi.mocked(useApplyTiering).mockReturnValue({
      mutate: vi.fn(),
      isPending: true,
      isError: false,
      error: null,
      variables: { confirmations: [{ project_id: 'proj-a', confirm_all: true }] },
    } as any)
    const project = makeProject({ project_id: 'proj-a', defs: [makeDef()] })
    vi.mocked(useTieringReport).mockReturnValue({
      isLoading: false,
      error: null,
      data: makeReport([project]),
    } as any)
    renderSection()

    expect(screen.getByRole('button', { name: 'Applying…' })).toBeDisabled()
  })

  it('disables the Apply button when no defs are applicable', () => {
    const project = makeProject({
      project_id: 'proj-a',
      defs: [makeDef({ skip_reason: 'consultant' })],
    })
    vi.mocked(useTieringReport).mockReturnValue({
      isLoading: false,
      error: null,
      data: makeReport([project]),
    } as any)
    renderSection()

    expect(screen.getByRole('button', { name: 'Apply (0)' })).toBeDisabled()
  })
})
