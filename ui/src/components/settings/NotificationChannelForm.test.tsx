import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useState } from 'react'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  NotificationChannelForm,
  emptyChannelForm,
  channelToFormData,
  buildConfig,
  type ChannelFormData,
} from './NotificationChannelForm'
import * as notificationsApi from '@/api/notifications'
import { renderWithQuery } from '@/test/utils'
import type { NotificationChannel } from '@/types/notifications'

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

function FormWrapper({
  initialData = emptyChannelForm(),
  editingChannel,
  isCreate = false,
  onSave = vi.fn(),
  onCancel = vi.fn(),
}: {
  initialData?: ChannelFormData
  editingChannel?: NotificationChannel
  isCreate?: boolean
  onSave?: () => void
  onCancel?: () => void
}) {
  const [formData, setFormData] = useState(initialData)
  return (
    <NotificationChannelForm
      formData={formData}
      setFormData={setFormData}
      onCancel={onCancel}
      onSave={onSave}
      mutation={{ isPending: false, isError: false, error: null }}
      isCreate={isCreate}
      editingChannel={editingChannel}
    />
  )
}

describe('NotificationChannelForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(notificationsApi.listNotificationDeliveries).mockResolvedValue([])
  })

  it('kind=slack shows webhook_url field; kind=telegram shows bot_token and chat_id', async () => {
    renderWithQuery(<FormWrapper isCreate initialData={{ ...emptyChannelForm(), kind: 'slack' }} />)

    expect(screen.getByPlaceholderText('https://hooks.slack.com/services/...')).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('123456:ABC-DEF...')).not.toBeInTheDocument()
    expect(screen.queryByPlaceholderText('-1001234567890')).not.toBeInTheDocument()

    // Switch to Telegram via the Dropdown
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Slack/ }))
    await user.click(screen.getByText('Telegram'))

    expect(screen.queryByPlaceholderText('https://hooks.slack.com/services/...')).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText('123456:ABC-DEF...')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('-1001234567890')).toBeInTheDocument()
  })

  it('edit mode pre-populates masked secret value in webhook_url input', async () => {
    const ch = makeChannel({
      id: 10,
      kind: 'slack',
      config: JSON.stringify({ webhook_url: '***masked***' }),
    })
    renderWithQuery(<FormWrapper editingChannel={ch} initialData={channelToFormData(ch)} />)
    expect(screen.getByDisplayValue('***masked***')).toBeInTheDocument()
  })

  it('channelToFormData preserves masked secret and buildConfig round-trips it', () => {
    const ch = makeChannel({ kind: 'slack', config: JSON.stringify({ webhook_url: '***secret***' }) })
    const formData = channelToFormData(ch)
    expect(formData.webhookUrl).toBe('***secret***')
    expect(buildConfig(formData)).toBe(JSON.stringify({ webhook_url: '***secret***' }))
  })

  it('typing in webhook_url replaces the masked value', async () => {
    const ch = makeChannel({ kind: 'slack', config: JSON.stringify({ webhook_url: '***' }) })
    renderWithQuery(<FormWrapper editingChannel={ch} initialData={channelToFormData(ch)} />)

    const user = userEvent.setup()
    const input = screen.getByDisplayValue('***')
    await user.clear(input)
    await user.type(input, 'https://new-webhook.example.com')

    expect(screen.getByDisplayValue('https://new-webhook.example.com')).toBeInTheDocument()
  })

  it('save disabled when no event types selected; checking one enables it', async () => {
    renderWithQuery(
      <FormWrapper
        isCreate
        initialData={{ ...emptyChannelForm(), name: 'mychan', eventTypes: [] }}
      />
    )

    const saveBtn = screen.getByRole('button', { name: 'Create' })
    expect(saveBtn).toBeDisabled()

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Workflow Completed' }))
    expect(saveBtn).not.toBeDisabled()
  })

  it('unchecking the last event type re-disables save', async () => {
    renderWithQuery(
      <FormWrapper
        isCreate
        initialData={{ ...emptyChannelForm(), name: 'mychan', eventTypes: ['workflow.completed'] }}
      />
    )

    const saveBtn = screen.getByRole('button', { name: 'Create' })
    expect(saveBtn).not.toBeDisabled()

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Workflow Completed' }))
    expect(saveBtn).toBeDisabled()
    expect(screen.getByText('Select at least one event type.')).toBeInTheDocument()
  })

  it('Send Test button is only shown in edit mode and calls testNotificationChannel', async () => {
    // Create mode: no Send Test button
    renderWithQuery(
      <FormWrapper isCreate initialData={{ ...emptyChannelForm(), eventTypes: ['workflow.completed'] }} />
    )
    expect(screen.queryByRole('button', { name: /Send Test/ })).not.toBeInTheDocument()
  })

  it('Send Test button in edit mode calls testNotificationChannel with channel id', async () => {
    vi.mocked(notificationsApi.testNotificationChannel).mockResolvedValue(undefined as never)

    const ch = makeChannel({ id: 99, event_types: ['workflow.completed'] })
    renderWithQuery(<FormWrapper editingChannel={ch} initialData={channelToFormData(ch)} />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Send Test/ }))

    await waitFor(() => {
      expect(notificationsApi.testNotificationChannel).toHaveBeenCalledWith(99)
    })
  })

  it('deliveries panel is hidden when no deliveries returned', async () => {
    vi.mocked(notificationsApi.listNotificationDeliveries).mockResolvedValue([])
    const ch = makeChannel({ id: 2 })
    renderWithQuery(<FormWrapper editingChannel={ch} initialData={channelToFormData(ch)} />)
    // Wait for deliveries query to settle
    await screen.findByRole('button', { name: /Send Test/ })
    expect(screen.queryByText('Recent Deliveries')).not.toBeInTheDocument()
  })
})
