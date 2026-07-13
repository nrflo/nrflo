import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunWorkflowForm } from './RunWorkflowForm'
import { renderWithQuery } from '@/test/utils'
import * as agentDefsApi from '@/api/agentDefs'
import * as settingsApi from '@/api/settings'
import type { GlobalSettings } from '@/api/settings'

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn(),
}))

vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return { ...actual, getGlobalSettings: vi.fn() }
})

vi.mock('@/components/workflow/ArtifactUploader', () => ({
  ArtifactUploader: () => <div data-testid="artifact-uploader" />,
}))

vi.mock('@/hooks/usePlan', () => ({
  useStartDynamicWorkflow: vi.fn(),
}))

import { useStartDynamicWorkflow } from '@/hooks/usePlan'

function makeSettings(overrides: Partial<GlobalSettings> = {}): GlobalSettings {
  return {
    api_mode_enabled: false,
    api_via_cli_enabled: false,
    claude_system_prompt_override_enabled: false,
    low_consumption_mode: false,
    context_save_via_agent: false,
    simplified_agents_graph: false,
    experimental: false,
    capture_thinking_enabled: false,
    experimental_observer_enabled: false,
    observer_system_context: '',
    observer_provider: '',
    observer_model: '',
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    menu_new_ticket: true,
    menu_import_spec: true,
    menu_git: true,
    menu_chain_executions: true,
    menu_schedules: true,
    menu_workflow_chains: true,
    menu_python_scripts: true,
    menu_documentation: true,
    menu_errors: true,
    menu_agent_sessions: true,
    dynamic_workflow_auto_enabled: false,
    ...overrides,
  }
}

const workflows: [string, { description: string; scope_type?: string; phases: [] }][] = [
  ['feature', { description: 'Feature workflow', scope_type: 'project', phases: [] }],
]

function renderForm({
  autoEnabled = false,
  projectId = 'proj-1',
  onDynamicRunSuccess = vi.fn(),
  mutate = vi.fn(),
  isPending = false,
  isError = false,
  error = null as Error | null,
}: {
  autoEnabled?: boolean
  projectId?: string | undefined
  onDynamicRunSuccess?: (id: string) => void
  mutate?: (...args: any[]) => void
  isPending?: boolean
  isError?: boolean
  error?: Error | null
} = {}) {
  vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([])
  vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ dynamic_workflow_auto_enabled: autoEnabled }))
  vi.mocked(useStartDynamicWorkflow).mockReturnValue({ mutate, isPending, isError, error } as any)

  return renderWithQuery(
    <RunWorkflowForm
      projectWorkflows={workflows}
      defsLoading={false}
      selectedWorkflowDef=""
      onSelectWorkflowDef={vi.fn()}
      instructions=""
      onInstructionsChange={vi.fn()}
      onRun={vi.fn()}
      runPending={false}
      runError={null}
      onStagedArtifactsChange={vi.fn()}
      hasUploadPending={false}
      onUploadPendingChange={vi.fn()}
      projectId={projectId}
      onDynamicRunSuccess={onDynamicRunSuccess}
    />
  )
}

describe('RunWorkflowForm — dynamic (planned) run', () => {
  beforeEach(() => vi.clearAllMocks())

  it('auto-approve toggle is disabled with a hint when the global gate is off', async () => {
    renderForm({ autoEnabled: false })

    const toggle = await screen.findByRole('switch', { name: /auto-approve plan/i })
    expect(toggle).toBeDisabled()
    expect(
      screen.getByText(/Enable "Allow dynamic_workflow mode=auto" in Settings to skip manual review/)
    ).toBeInTheDocument()
  })

  it('auto-approve toggle is enabled with no hint when the global gate is on', async () => {
    renderForm({ autoEnabled: true })

    await waitFor(() =>
      expect(screen.getByRole('switch', { name: /auto-approve plan/i })).not.toBeDisabled()
    )
    expect(screen.queryByText(/skip manual review/)).not.toBeInTheDocument()
  })

  it('submits {instructions, mode: "approve"} when auto-approve is off', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    renderForm({ autoEnabled: false, mutate })

    await user.type(
      screen.getByPlaceholderText(/describe the goal for the planner/i),
      'Build a login page'
    )
    await user.click(screen.getByRole('button', { name: /start dynamic run/i }))

    expect(mutate).toHaveBeenCalledWith(
      {
        projectId: 'proj-1',
        params: { instructions: 'Build a login page', mode: 'approve' },
      },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('submits mode: "auto" when the gate is on and the toggle is checked', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    renderForm({ autoEnabled: true, mutate })

    await user.type(
      screen.getByPlaceholderText(/describe the goal for the planner/i),
      'Build a login page'
    )
    await waitFor(() =>
      expect(screen.getByRole('switch', { name: /auto-approve plan/i })).not.toBeDisabled()
    )
    await user.click(screen.getByRole('switch', { name: /auto-approve plan/i }))
    await user.click(screen.getByRole('button', { name: /start dynamic run/i }))

    expect(mutate).toHaveBeenCalledWith(
      {
        projectId: 'proj-1',
        params: { instructions: 'Build a login page', mode: 'auto' },
      },
      expect.anything()
    )
  })

  it('Start Dynamic Run is disabled until instructions are non-blank', async () => {
    renderForm()
    expect(screen.getByRole('button', { name: /start dynamic run/i })).toBeDisabled()
  })

  it('on success, the returned instance_id is passed to onDynamicRunSuccess', async () => {
    const user = userEvent.setup()
    const onDynamicRunSuccess = vi.fn()
    const mutate = vi.fn((_vars, opts) => opts.onSuccess({ instance_id: 'inst-42', status: 'planning' }))
    renderForm({ mutate, onDynamicRunSuccess })

    await user.type(
      screen.getByPlaceholderText(/describe the goal for the planner/i),
      'Build a login page'
    )
    await user.click(screen.getByRole('button', { name: /start dynamic run/i }))

    expect(onDynamicRunSuccess).toHaveBeenCalledWith('inst-42')
  })

  it('shows the mutation error message when the dynamic run fails to start', async () => {
    renderForm({ isError: true, error: new Error('planner unavailable') })
    expect(await screen.findByText('planner unavailable')).toBeInTheDocument()
  })

  it('shows a pending spinner and disables the button while the run is starting', async () => {
    renderForm({ isPending: true })
    await waitFor(() => expect(screen.getByRole('button', { name: /start dynamic run/i })).toBeDisabled())
  })
})
