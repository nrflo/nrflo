import { describe, it, expect, vi } from 'vitest'
import React, { useState } from 'react'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NotificationTemplateEditor } from './NotificationTemplateEditor'
import { renderWithQuery } from '@/test/utils'

// The mock makes insertAtCaret append text to current value and call onChange,
// so chip-click tests work end-to-end through the controlled value.
vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: React.forwardRef(
    (
      { value, onChange }: { value: string; onChange?: (v: string) => void; placeholder?: string; readOnly?: boolean; minHeight?: string; maxHeight?: string },
      ref: React.Ref<{ insertAtCaret: (text: string) => void }>,
    ) => {
      const valueRef = React.useRef(value)
      valueRef.current = value
      React.useImperativeHandle(ref, () => ({
        insertAtCaret(text: string) {
          onChange?.(valueRef.current + text)
        },
      }))
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

function Wrapper({
  initialValue,
  variables,
  onChangeSpy,
}: {
  initialValue: string
  variables: string[]
  onChangeSpy: (v: string) => void
}) {
  const [val, setVal] = useState(initialValue)
  return (
    <NotificationTemplateEditor
      kind="slack"
      value={val}
      onChange={(v) => { setVal(v); onChangeSpy(v) }}
      variables={variables}
    />
  )
}

describe('NotificationTemplateEditor', () => {
  it('chip click appends ${varName} at caret and calls onChange with updated value', async () => {
    const onChangeSpy = vi.fn()
    const user = userEvent.setup()

    renderWithQuery(
      <Wrapper initialValue="prefix " variables={['event_type', 'workflow']} onChangeSpy={onChangeSpy} />
    )

    await user.click(screen.getByRole('button', { name: '${event_type}' }))

    expect(onChangeSpy).toHaveBeenCalledWith('prefix ${event_type}')
    expect(screen.getByTestId('markdown-editor')).toHaveValue('prefix ${event_type}')
  })

  it('preview substitutes known variables and removes unknown ones', () => {
    renderWithQuery(
      <NotificationTemplateEditor
        kind="slack"
        value="hi ${ticket_name} ${unknown_var}"
        onChange={vi.fn()}
        variables={[]}
      />
    )
    // ticket_name maps to 'Add user authentication' in SAMPLE; unknown_var → ''
    expect(screen.getByText(/hi Add user authentication/)).toBeInTheDocument()
  })

  it('shows structure-only caption regardless of template content', () => {
    renderWithQuery(
      <NotificationTemplateEditor
        kind="slack"
        value=""
        onChange={vi.fn()}
        variables={[]}
      />
    )
    expect(screen.getByText(/structure-only/i)).toBeInTheDocument()
  })
})
