import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NotificationsSection } from './NotificationsSection'
import * as notificationsApi from '@/api/notifications'
import { renderWithQuery } from '@/test/utils'
import type { NotificationChannel } from '@/types/notifications'
import { toast } from 'sonner'

vi.mock('@/api/notifications')
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

function makeChannel(overrides: Partial<NotificationChannel> = {}): NotificationChannel {
  return {
    id: 1,
    project_id: 'test-project',
    name: 'My Channel',
    kind: 'slack',
    enabled: true,
    config: JSON.stringify({ webhook_url: '***' }),
    event_types: ['workflow.completed'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('NotificationsSection', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows empty state when no channels', async () => {
    vi.mocked(notificationsApi.listNotificationChannels).mockResolvedValue([])
    renderWithQuery(<NotificationsSection />)
    expect(
      await screen.findByText('No notification channels configured. Create one to get started.')
    ).toBeInTheDocument()
  })

  it('renders channel list with kind badge, name, and event-type chips', async () => {
    vi.mocked(notificationsApi.listNotificationChannels).mockResolvedValue([
      makeChannel({
        name: 'Alerts',
        kind: 'slack',
        event_types: ['workflow.completed', 'workflow.failed'],
      }),
    ])
    renderWithQuery(<NotificationsSection />)

    expect(await screen.findByText('Alerts')).toBeInTheDocument()
    expect(screen.getByText('Slack')).toBeInTheDocument()
    expect(screen.getByText('workflow.completed')).toBeInTheDocument()
    expect(screen.getByText('workflow.failed')).toBeInTheDocument()
  })

  it('telegram channel shows Telegram badge', async () => {
    vi.mocked(notificationsApi.listNotificationChannels).mockResolvedValue([
      makeChannel({ kind: 'telegram', config: JSON.stringify({ bot_token: '***', chat_id: '-1001' }) }),
    ])
    renderWithQuery(<NotificationsSection />)
    expect(await screen.findByText('Telegram')).toBeInTheDocument()
  })

  it('enabled toggle click calls updateNotificationChannel with enabled:false', async () => {
    vi.mocked(notificationsApi.listNotificationChannels).mockResolvedValue([
      makeChannel({ id: 42, enabled: true }),
    ])
    vi.mocked(notificationsApi.updateNotificationChannel).mockResolvedValue(
      makeChannel({ id: 42, enabled: false })
    )
    renderWithQuery(<NotificationsSection />)
    await screen.findByText('My Channel')

    const user = userEvent.setup()
    await user.click(screen.getByRole('switch'))

    await waitFor(() => {
      expect(notificationsApi.updateNotificationChannel).toHaveBeenCalledWith(42, { enabled: false })
    })
  })

  it('delete: Cancel dismisses without calling API, then Delete confirms', async () => {
    vi.mocked(notificationsApi.listNotificationChannels)
      .mockResolvedValueOnce([makeChannel({ id: 7, name: 'Test Channel' })])
      .mockResolvedValue([])
    vi.mocked(notificationsApi.deleteNotificationChannel).mockResolvedValue(undefined as never)

    renderWithQuery(<NotificationsSection />)
    await screen.findByText('Test Channel')

    const user = userEvent.setup()
    // trash is the last button in the row (after New Channel, Pencil, Send)
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[buttons.length - 1])

    expect(screen.getByText(/Are you sure you want to delete/)).toBeInTheDocument()

    // Cancel dismisses
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText(/Are you sure you want to delete/)).not.toBeInTheDocument()
    expect(notificationsApi.deleteNotificationChannel).not.toHaveBeenCalled()

    // Re-open and confirm
    const buttons2 = screen.getAllByRole('button')
    await user.click(buttons2[buttons2.length - 1])
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(notificationsApi.deleteNotificationChannel).toHaveBeenCalledWith(7)
    })
  })

  it('Send Test button calls testNotificationChannel and shows success toast', async () => {
    vi.mocked(notificationsApi.listNotificationChannels).mockResolvedValue([
      makeChannel({ id: 5 }),
    ])
    vi.mocked(notificationsApi.testNotificationChannel).mockResolvedValue(undefined as never)

    renderWithQuery(<NotificationsSection />)
    await screen.findByText('My Channel')

    const user = userEvent.setup()
    await user.click(screen.getByTitle('Send test notification'))

    await waitFor(() => {
      expect(notificationsApi.testNotificationChannel).toHaveBeenCalledWith(5)
      expect(toast.success).toHaveBeenCalledWith('Test notification sent')
    })
  })

  it('New Channel button is disabled while create form is open', async () => {
    vi.mocked(notificationsApi.listNotificationChannels).mockResolvedValue([])
    renderWithQuery(<NotificationsSection />)
    await screen.findByText('No notification channels configured. Create one to get started.')

    const user = userEvent.setup()
    const newBtn = screen.getByRole('button', { name: /New Channel/ })
    await user.click(newBtn)
    expect(newBtn).toBeDisabled()
  })
})
