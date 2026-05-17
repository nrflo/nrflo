import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RunWorkflowDialog } from './RunWorkflowDialog'
import type { InputArtifactRef } from '@/types/artifact'

// ── Mocks ────────────────────────────────────────────────────────────────────

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (sel: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    sel({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn().mockResolvedValue({
    'basic-wf': {
      description: 'Basic workflow',
      scope_type: 'ticket',
      phases: [{ id: 'impl', agent: 'impl', layer: 0 }],
    },
  }),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn().mockResolvedValue([]),
}))

const mockMutateAsync = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useRunWorkflow: vi.fn().mockReturnValue({
    mutateAsync: (...args: unknown[]) => mockMutateAsync(...args),
    isPending: false,
    isError: false,
    error: null,
  }),
}))

const mockCancelUpload = vi.fn()
vi.mock('@/api/artifacts', () => ({
  cancelUpload: (...args: unknown[]) => mockCancelUpload(...args),
}))

// Controllable stub for ArtifactUploader
let uploaderOnChange: ((refs: InputArtifactRef[], hasPending: boolean) => void) | null = null
vi.mock('@/components/workflow/ArtifactUploader', () => ({
  ArtifactUploader: ({ onChange }: { onChange: (refs: InputArtifactRef[], hasPending: boolean) => void }) => {
    uploaderOnChange = onChange
    return (
      <div data-testid="artifact-uploader">
        <button type="button" onClick={() => onChange([], true)}>sim-pending</button>
        <button type="button" onClick={() => onChange([{ upload_id: 'uid-1', name: 'f.txt' }], false)}>
          sim-done
        </button>
      </div>
    )
  },
}))

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderDialog(overrides: Partial<React.ComponentProps<typeof RunWorkflowDialog>> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <RunWorkflowDialog open={true} onClose={vi.fn()} ticketId="tick-1" {...overrides} />
    </QueryClientProvider>
  )
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('RunWorkflowDialog — artifact integration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    uploaderOnChange = null
    mockCancelUpload.mockResolvedValue(undefined)
    mockMutateAsync.mockResolvedValue({ session_id: null, instance_id: 'inst-1' })
  })

  it('Run button is disabled while upload is pending', async () => {
    renderDialog()
    await waitFor(() => expect(screen.getByRole('button', { name: /run/i })).not.toBeDisabled())

    await userEvent.click(screen.getByRole('button', { name: 'sim-pending' }))

    expect(screen.getByRole('button', { name: /run/i })).toBeDisabled()
  })

  it('Run button is re-enabled after upload completes', async () => {
    renderDialog()
    await waitFor(() => expect(screen.getByRole('button', { name: /run/i })).not.toBeDisabled())

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'sim-pending' }))
    expect(screen.getByRole('button', { name: /run/i })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'sim-done' }))
    expect(screen.getByRole('button', { name: /run/i })).not.toBeDisabled()
  })

  it('includes input_artifacts in mutateAsync call when artifacts are staged', async () => {
    const user = userEvent.setup()
    renderDialog()
    await waitFor(() => expect(screen.getByRole('button', { name: /run/i })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: 'sim-done' }))
    await user.click(screen.getByRole('button', { name: /run/i }))

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalled())
    const callArgs = mockMutateAsync.mock.calls[0][0]
    expect(callArgs.params.input_artifacts).toEqual([{ upload_id: 'uid-1', name: 'f.txt' }])
  })

  it('does not include input_artifacts when no artifacts staged', async () => {
    const user = userEvent.setup()
    renderDialog()
    await waitFor(() => expect(screen.getByRole('button', { name: /run/i })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: /run/i }))

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalled())
    const callArgs = mockMutateAsync.mock.calls[0][0]
    expect(callArgs.params.input_artifacts).toBeUndefined()
  })

  it('calls cancelUpload for staged uploads when dialog closes without launching', async () => {
    const user = userEvent.setup()
    const { rerender } = renderDialog({ open: true, onClose: vi.fn() })
    await waitFor(() => expect(screen.getByRole('button', { name: /run/i })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: 'sim-done' }))

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    rerender(
      <QueryClientProvider client={queryClient}>
        <RunWorkflowDialog open={false} onClose={vi.fn()} ticketId="tick-1" />
      </QueryClientProvider>
    )

    await waitFor(() => {
      expect(mockCancelUpload).toHaveBeenCalledWith('uid-1')
    })
  })
})
