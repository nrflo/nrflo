import { describe, it, expect, vi, beforeEach } from 'vitest'
import React, { useState } from 'react'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  NotificationChannelForm,
  emptyChannelForm,
  channelToFormData,
  buildConfig,
  type ChannelFormData,
} from './NotificationChannelForm'
import * as api from '@/api/notifications'
import { renderWithQuery } from '@/test/utils'
import type { NotificationChannel } from '@/types/notifications'

vi.mock('@/api/notifications')

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: React.forwardRef(
    (
      { value, onChange, placeholder }: { value: string; onChange?: (v: string) => void; placeholder?: string; readOnly?: boolean; minHeight?: string; maxHeight?: string },
      ref: React.Ref<{ insertAtCaret: (text: string) => void }>,
    ) => {
      React.useImperativeHandle(ref, () => ({ insertAtCaret: vi.fn() }))
      return <textarea value={value} onChange={(e) => onChange?.(e.target.value)} placeholder={placeholder} data-testid="markdown-editor" readOnly={!onChange} />
    },
  ),
}))

const WF_ID = 'wf-form-test'

function makeChannel(overrides: Partial<NotificationChannel> = {}): NotificationChannel {
  return {
    id: 'ch-99',
    project_id: 'proj-1',
    workflow_id: WF_ID,
    name: '#edit-me',
    kind: 'slack',
    enabled: true,
    config: JSON.stringify({ webhook_url: 'https://hooks.slack.com/masked' }),
    event_types: ['orchestration.completed'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const noopMutation = { isPending: false, isError: false, error: null }

// Stateful wrapper so form state changes re-render correctly
function TestForm({
  initial = emptyChannelForm(),
  isCreate,
  editingChannel,
  onSave = vi.fn(),
  onCancel = vi.fn(),
}: {
  initial?: ChannelFormData
  isCreate?: boolean
  editingChannel?: NotificationChannel
  onSave?: () => void
  onCancel?: () => void
}) {
  const [formData, setFormData] = useState<ChannelFormData>(initial)
  return (
    <NotificationChannelForm
      workflowId={WF_ID}
      formData={formData}
      setFormData={setFormData}
      onCancel={onCancel}
      onSave={onSave}
      mutation={noopMutation}
      isCreate={isCreate}
      editingChannel={editingChannel}
    />
  )
}

describe('channelToFormData + buildConfig round-trip', () => {
  it('extracts slack webhook_url from config JSON', () => {
    const ch = makeChannel({ kind: 'slack', config: JSON.stringify({ webhook_url: 'https://example.com/hook' }) })
    const fd = channelToFormData(ch)
    expect(fd.webhookUrl).toBe('https://example.com/hook')
    expect(fd.botToken).toBe('')
    expect(fd.chatId).toBe('')
    expect(fd.kind).toBe('slack')
    expect(fd.name).toBe('#edit-me')
  })

  it('extracts telegram bot_token and chat_id from config JSON', () => {
    const ch = makeChannel({
      kind: 'telegram',
      config: JSON.stringify({ bot_token: 'abc:def', chat_id: '-100123' }),
    })
    const fd = channelToFormData(ch)
    expect(fd.botToken).toBe('abc:def')
    expect(fd.chatId).toBe('-100123')
    expect(fd.webhookUrl).toBe('')
  })

  it('buildConfig produces correct object for slack', () => {
    const fd: ChannelFormData = { ...emptyChannelForm(), kind: 'slack', webhookUrl: 'https://x.com/h' }
    expect(buildConfig(fd)).toEqual({ webhook_url: 'https://x.com/h' })
  })

  it('buildConfig produces correct object for telegram', () => {
    const fd: ChannelFormData = { ...emptyChannelForm(), kind: 'telegram', botToken: 'tok', chatId: '-9' }
    expect(buildConfig(fd)).toEqual({ bot_token: 'tok', chat_id: '-9' })
  })
})

describe('NotificationChannelForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.listNotificationDeliveries).mockResolvedValue([])
    vi.mocked(api.getNotificationVariables).mockResolvedValue({ variables: [], defaults: { slack: '', telegram: '' } })
  })

  it('create mode: shows webhook URL field for slack, Create button, no Send Test', () => {
    renderWithQuery(<TestForm isCreate />)
    expect(screen.getByPlaceholderText('https://hooks.slack.com/services/...')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Send Test/i })).not.toBeInTheDocument()
  })

  it('create mode: shows bot_token and chat_id fields when kind is switched to telegram', async () => {
    renderWithQuery(<TestForm isCreate />)
    const user = userEvent.setup()

    // Open kind dropdown (trigger shows "Slack")
    await user.click(screen.getByRole('button', { name: /Slack/i }))
    await user.click(screen.getByText('Telegram'))

    expect(screen.getByPlaceholderText('123456:ABC-DEF...')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('-1001234567890')).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('https://hooks.slack.com/services/...')).not.toBeInTheDocument()
  })

  it('save disabled when no event types selected, enabled when name+event present', () => {
    renderWithQuery(
      <TestForm isCreate initial={{ ...emptyChannelForm(), name: 'my-channel', eventTypes: [] }} />
    )
    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  })

  it('save enabled after selecting event type, re-disabled after deselecting last one', async () => {
    renderWithQuery(
      <TestForm isCreate initial={{ ...emptyChannelForm(), name: 'my-channel', eventTypes: [] }} />
    )
    const user = userEvent.setup()
    const createBtn = screen.getByRole('button', { name: 'Create' })

    expect(createBtn).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Workflow Completed' }))
    expect(createBtn).not.toBeDisabled()

    // Deselect the only event type → save becomes disabled again
    await user.click(screen.getByRole('button', { name: 'Workflow Completed' }))
    expect(createBtn).toBeDisabled()
  })

  it('edit mode: Send Test button visible, kind shown as disabled input', async () => {
    renderWithQuery(
      <TestForm
        editingChannel={makeChannel({ kind: 'telegram', config: JSON.stringify({ bot_token: 't', chat_id: '1' }) })}
        initial={channelToFormData(makeChannel({ kind: 'telegram', config: JSON.stringify({ bot_token: 't', chat_id: '1' }) }))}
      />
    )
    // Send Test visible in edit mode
    expect(screen.getByRole('button', { name: /Send Test/i })).toBeInTheDocument()
    // Kind is displayed as a disabled input (not a dropdown)
    const kindInput = screen.getByDisplayValue('telegram')
    expect(kindInput).toBeDisabled()
  })

  it('edit mode: Send Test calls testNotificationChannel with workflowId and channelId', async () => {
    vi.mocked(api.testNotificationChannel).mockResolvedValue()
    const ch = makeChannel({ id: 'ch-99' })

    renderWithQuery(
      <TestForm
        editingChannel={ch}
        initial={channelToFormData(ch)}
      />
    )
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Send Test/i }))

    await waitFor(() => {
      expect(api.testNotificationChannel).toHaveBeenCalledWith(WF_ID, 'ch-99')
    })
  })

  it('deliveries panel hidden when deliveries list is empty', async () => {
    vi.mocked(api.listNotificationDeliveries).mockResolvedValue([])
    const ch = makeChannel()

    renderWithQuery(
      <TestForm editingChannel={ch} initial={channelToFormData(ch)} />
    )
    // Give queries time to resolve
    await screen.findByRole('button', { name: /Send Test/i })
    expect(screen.queryByText('Recent Deliveries')).not.toBeInTheDocument()
  })

  it('deliveries panel renders table when deliveries are present', async () => {
    vi.mocked(api.listNotificationDeliveries).mockResolvedValue([
      {
        id: 1,
        channel_id: 'ch-99',
        event_type: 'orchestration.completed',
        status: 'delivered',
        attempts: 1,
        last_error: '',
        next_attempt_at: null,
        created_at: '2026-01-01T00:00:00Z',
      },
    ])
    const ch = makeChannel()

    renderWithQuery(
      <TestForm editingChannel={ch} initial={channelToFormData(ch)} />
    )
    expect(await screen.findByText('Recent Deliveries')).toBeInTheDocument()
    expect(screen.getByText('orchestration.completed')).toBeInTheDocument()
    expect(screen.getByText('delivered')).toBeInTheDocument()
  })

  it('masked secret: channelToFormData reads webhook_url from config into webhookUrl field', () => {
    const ch = makeChannel({ config: JSON.stringify({ webhook_url: 'https://hooks.slack.com/secret' }) })
    const fd = channelToFormData(ch)
    renderWithQuery(
      <NotificationChannelForm
        workflowId={WF_ID}
        formData={fd}
        setFormData={vi.fn()}
        onCancel={vi.fn()}
        onSave={vi.fn()}
        mutation={noopMutation}
        editingChannel={ch}
      />
    )
    // The webhook_url input (type=password) should show the pre-populated value
    const inputs = screen.getAllByDisplayValue('https://hooks.slack.com/secret')
    expect(inputs.length).toBeGreaterThan(0)
  })
})
