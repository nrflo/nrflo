import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowNotificationsSection } from './WorkflowNotificationsSection'
import * as api from '@/api/notifications'
import { renderWithQuery } from '@/test/utils'
import type { NotificationChannel } from '@/types/notifications'

vi.mock('@/api/notifications')

const WF_ID = 'wf-abc-123'

function makeChannel(overrides: Partial<NotificationChannel> = {}): NotificationChannel {
  return {
    id: 'ch-1',
    project_id: 'proj-1',
    workflow_id: WF_ID,
    name: '#alerts',
    kind: 'slack',
    enabled: true,
    config: JSON.stringify({ webhook_url: 'https://hooks.slack.com/xxx' }),
    event_types: ['workflow.completed', 'workflow.failed'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('WorkflowNotificationsSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.listNotificationDeliveries).mockResolvedValue([])
  })

  it('shows empty state and passes workflowId to listNotificationChannels', async () => {
    vi.mocked(api.listNotificationChannels).mockResolvedValue([])
    renderWithQuery(<WorkflowNotificationsSection workflowId={WF_ID} />)
    expect(
      await screen.findByText('No notification channels configured. Create one to get started.')
    ).toBeInTheDocument()
    expect(api.listNotificationChannels).toHaveBeenCalledWith(WF_ID)
  })

  it('renders channel list with name, slack kind badge, and event type chips', async () => {
    vi.mocked(api.listNotificationChannels).mockResolvedValue([makeChannel()])
    renderWithQuery(<WorkflowNotificationsSection workflowId={WF_ID} />)
    expect(await screen.findByText('#alerts')).toBeInTheDocument()
    expect(screen.getByText('Slack')).toBeInTheDocument()
    expect(screen.getByText('workflow.completed')).toBeInTheDocument()
    expect(screen.getByText('workflow.failed')).toBeInTheDocument()
  })

  it('shows Telegram badge for telegram kind channels', async () => {
    vi.mocked(api.listNotificationChannels).mockResolvedValue([
      makeChannel({ kind: 'telegram', config: JSON.stringify({ bot_token: 'tok', chat_id: '123' }) }),
    ])
    renderWithQuery(<WorkflowNotificationsSection workflowId={WF_ID} />)
    expect(await screen.findByText('Telegram')).toBeInTheDocument()
  })

  it('toggle calls updateNotificationChannel with workflowId and toggled enabled value', async () => {
    vi.mocked(api.listNotificationChannels).mockResolvedValue([
      makeChannel({ id: 'ch-1', enabled: true }),
    ])
    vi.mocked(api.updateNotificationChannel).mockResolvedValue(makeChannel({ enabled: false }))

    renderWithQuery(<WorkflowNotificationsSection workflowId={WF_ID} />)
    await screen.findByText('#alerts')

    const user = userEvent.setup()
    // Toggle renders as role="switch"
    await user.click(screen.getByRole('switch'))

    await waitFor(() => {
      expect(api.updateNotificationChannel).toHaveBeenCalledWith(WF_ID, 'ch-1', { enabled: false })
    })
  })

  it('delete: cancel dismisses confirmation, confirm calls deleteNotificationChannel with workflowId+id', async () => {
    vi.mocked(api.listNotificationChannels).mockResolvedValue([
      makeChannel({ id: 'ch-1', name: '#alerts' }),
    ])
    vi.mocked(api.deleteNotificationChannel).mockResolvedValue()

    renderWithQuery(<WorkflowNotificationsSection workflowId={WF_ID} />)
    await screen.findByText('#alerts')

    const user = userEvent.setup()
    // Row buttons: [0]=New Channel, [1]=edit pencil, [2]=send test, [3]=delete trash
    let btns = screen.getAllByRole('button')
    await user.click(btns[3])

    expect(screen.getByText(/Are you sure you want to delete/)).toBeInTheDocument()
    expect(screen.getAllByText('#alerts').length).toBeGreaterThan(0)

    // Cancel dismisses without calling API
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText(/Are you sure you want to delete/)).not.toBeInTheDocument()
    expect(api.deleteNotificationChannel).not.toHaveBeenCalled()

    // Re-open and confirm
    btns = screen.getAllByRole('button')
    await user.click(btns[3])
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(api.deleteNotificationChannel).toHaveBeenCalledWith(WF_ID, 'ch-1')
    })
  })

  it('send test (list-level) calls testNotificationChannel with workflowId and id', async () => {
    vi.mocked(api.listNotificationChannels).mockResolvedValue([makeChannel({ id: 'ch-1' })])
    vi.mocked(api.testNotificationChannel).mockResolvedValue()

    renderWithQuery(<WorkflowNotificationsSection workflowId={WF_ID} />)
    await screen.findByText('#alerts')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Send test notification' }))

    await waitFor(() => {
      expect(api.testNotificationChannel).toHaveBeenCalledWith(WF_ID, 'ch-1')
    })
  })

  it('New Channel button is disabled while create form is open', async () => {
    vi.mocked(api.listNotificationChannels).mockResolvedValue([])

    renderWithQuery(<WorkflowNotificationsSection workflowId={WF_ID} />)
    await screen.findByText('No notification channels configured. Create one to get started.')

    const user = userEvent.setup()
    const newBtn = screen.getByRole('button', { name: /New Channel/ })
    await user.click(newBtn)

    expect(newBtn).toBeDisabled()
  })

  it('create calls createNotificationChannel with workflowId and form payload', async () => {
    vi.mocked(api.listNotificationChannels).mockResolvedValue([])
    vi.mocked(api.createNotificationChannel).mockResolvedValue(makeChannel({ name: '#my-ch' }))

    renderWithQuery(<WorkflowNotificationsSection workflowId={WF_ID} />)
    await screen.findByText('No notification channels configured. Create one to get started.')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /New Channel/ }))

    // Fill name
    await user.type(screen.getByPlaceholderText('e.g. #alerts'), '#my-ch')
    // Create is still disabled without event types
    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()

    // Select an event type
    await user.click(screen.getByRole('button', { name: 'Workflow Completed' }))
    expect(screen.getByRole('button', { name: 'Create' })).not.toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(api.createNotificationChannel).toHaveBeenCalledWith(
        WF_ID,
        expect.objectContaining({
          name: '#my-ch',
          kind: 'slack',
          event_types: ['workflow.completed'],
        })
      )
    })
  })
})
