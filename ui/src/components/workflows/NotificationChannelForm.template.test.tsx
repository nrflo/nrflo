import { describe, it, expect, vi, beforeEach } from 'vitest'
import React, { useState } from 'react'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  NotificationChannelForm,
  emptyChannelForm,
  type ChannelFormData,
} from './NotificationChannelForm'
import * as api from '@/api/notifications'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/notifications')

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: React.forwardRef(
    (
      { value, onChange }: { value: string; onChange?: (v: string) => void; placeholder?: string; readOnly?: boolean; minHeight?: string; maxHeight?: string },
      ref: React.Ref<{ insertAtCaret: (text: string) => void }>,
    ) => {
      React.useImperativeHandle(ref, () => ({ insertAtCaret: vi.fn() }))
      return (
        <textarea
          value={value}
          onChange={(e) => onChange?.(e.target.value)}
          data-testid="markdown-editor"
        />
      )
    },
  ),
}))

const WF_ID = 'wf-template-test'
const noopMutation = { isPending: false, isError: false, error: null }

function TestForm({ initial = emptyChannelForm() }: { initial?: ChannelFormData }) {
  const [formData, setFormData] = useState<ChannelFormData>(initial)
  return (
    <NotificationChannelForm
      workflowId={WF_ID}
      formData={formData}
      setFormData={setFormData}
      onCancel={vi.fn()}
      onSave={vi.fn()}
      mutation={noopMutation}
      isCreate
    />
  )
}

describe('NotificationChannelForm — template prefill and kind toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.listNotificationDeliveries).mockResolvedValue([])
  })

  it('create mode: prefills messageTemplate from defaults.slack when variables query resolves', async () => {
    vi.mocked(api.getNotificationVariables).mockResolvedValue({
      variables: ['event_type', 'workflow'],
      defaults: { slack: '**Slack default ${event_type}**', telegram: 'Telegram default' },
    })
    renderWithQuery(<TestForm />)
    expect(await screen.findByDisplayValue('**Slack default ${event_type}**')).toBeInTheDocument()
  })

  it('kind toggle when unedited: swaps template to new-kind default', async () => {
    vi.mocked(api.getNotificationVariables).mockResolvedValue({
      variables: [],
      defaults: { slack: 'slack-default', telegram: 'telegram-default' },
    })
    const user = userEvent.setup()
    renderWithQuery(<TestForm />)

    await screen.findByDisplayValue('slack-default')

    await user.click(screen.getByRole('button', { name: /Slack/i }))
    await user.click(screen.getByText('Telegram'))

    expect(screen.getByTestId('markdown-editor')).toHaveValue('telegram-default')
  })

  it('kind toggle preserves custom template and shows syntax hint', async () => {
    vi.mocked(api.getNotificationVariables).mockResolvedValue({
      variables: [],
      defaults: { slack: 'slack-default', telegram: 'telegram-default' },
    })
    const user = userEvent.setup()
    renderWithQuery(<TestForm />)

    await screen.findByDisplayValue('slack-default')

    const textarea = screen.getByTestId('markdown-editor')
    await user.clear(textarea)
    await user.type(textarea, 'custom template')

    await user.click(screen.getByRole('button', { name: /Slack/i }))
    await user.click(screen.getByText('Telegram'))

    expect(screen.getByTestId('markdown-editor')).toHaveValue('custom template')
    expect(screen.getByText(/Syntax may differ/i)).toBeInTheDocument()
  })
})
