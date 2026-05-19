import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LaunchObserverButton } from './LaunchObserverButton'
import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'
import * as observersApi from '@/api/observers'
import { renderWithQuery } from '@/test/utils'

// ---- settings mock ----
const mockObserverEnabled = vi.fn()

vi.mock('@/hooks/useGlobalSettings', () => ({
  useExperimentalObserverEnabled: () => mockObserverEnabled(),
}))

// ---- api mock ----
vi.mock('@/api/observers', () => ({
  launchObserver: vi.fn(),
  listObservers: vi.fn(),
}))

// ---- project store mock ----
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string }) => unknown) =>
    selector({ currentProject: 'test-project' }),
}))

describe('LaunchObserverButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useInteractiveSessionsStore.setState({ sessions: [], activeId: '', minimized: false })
  })

  it('renders nothing when experimental_observer_enabled is false', () => {
    mockObserverEnabled.mockReturnValue(false)
    const { container } = renderWithQuery(
      <LaunchObserverButton payload={{ scope: 'project', project_id: 'proj-1' }} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders Observer button when flag is enabled', () => {
    mockObserverEnabled.mockReturnValue(true)
    renderWithQuery(<LaunchObserverButton payload={{ scope: 'project', project_id: 'proj-1' }} />)
    expect(screen.getByRole('button', { name: /observer/i })).toBeInTheDocument()
  })

  it('clicking button calls launchObserver with project scope payload', async () => {
    mockObserverEnabled.mockReturnValue(true)
    vi.mocked(observersApi.launchObserver).mockResolvedValue({ session_id: 'abc-123' })

    const user = userEvent.setup()
    renderWithQuery(
      <LaunchObserverButton payload={{ scope: 'project', project_id: 'proj-x' }} />
    )

    await user.click(screen.getByRole('button', { name: /observer/i }))

    await waitFor(() => {
      expect(observersApi.launchObserver).toHaveBeenCalledWith({
        scope: 'project',
        project_id: 'proj-x',
      })
    })
  })

  it('clicking button calls launchObserver with global scope payload', async () => {
    mockObserverEnabled.mockReturnValue(true)
    vi.mocked(observersApi.launchObserver).mockResolvedValue({ session_id: 'abc-456' })

    const user = userEvent.setup()
    renderWithQuery(<LaunchObserverButton payload={{ scope: 'global' }} />)

    await user.click(screen.getByRole('button', { name: /observer/i }))

    await waitFor(() => {
      expect(observersApi.launchObserver).toHaveBeenCalledWith({ scope: 'global' })
    })
  })

  it('clicking button calls launchObserver with workflow scope payload', async () => {
    mockObserverEnabled.mockReturnValue(true)
    vi.mocked(observersApi.launchObserver).mockResolvedValue({ session_id: 'abc-789' })

    const user = userEvent.setup()
    renderWithQuery(
      <LaunchObserverButton
        payload={{ scope: 'workflow', project_id: 'proj-1', workflow_id: 'feature' }}
      />
    )

    await user.click(screen.getByRole('button', { name: /observer/i }))

    await waitFor(() => {
      expect(observersApi.launchObserver).toHaveBeenCalledWith({
        scope: 'workflow',
        project_id: 'proj-1',
        workflow_id: 'feature',
      })
    })
  })

  it('on success, session_id is added to interactiveSessionsStore', async () => {
    mockObserverEnabled.mockReturnValue(true)
    vi.mocked(observersApi.launchObserver).mockResolvedValue({ session_id: 'new-session-id' })

    const user = userEvent.setup()
    renderWithQuery(
      <LaunchObserverButton payload={{ scope: 'project', project_id: 'proj-1' }} />
    )

    await user.click(screen.getByRole('button', { name: /observer/i }))

    await waitFor(() => {
      const sessions = useInteractiveSessionsStore.getState().sessions
      expect(sessions).toContainEqual(
        expect.objectContaining({ sessionId: 'new-session-id', agentType: 'observer' })
      )
    })
  })

  it('button is disabled while mutation is pending', async () => {
    mockObserverEnabled.mockReturnValue(true)
    // Never resolves — keeps isPending true
    vi.mocked(observersApi.launchObserver).mockReturnValue(new Promise(() => {}))

    const user = userEvent.setup()
    renderWithQuery(
      <LaunchObserverButton payload={{ scope: 'project', project_id: 'proj-1' }} />
    )

    const btn = screen.getByRole('button', { name: /observer/i })
    await user.click(btn)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /observer/i })).toBeDisabled()
    })
  })
})
