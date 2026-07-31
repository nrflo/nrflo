import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MessageTableRow } from './MessageTableRow'
import type { MessageWithTime } from '@/types/workflow'

function renderRow(msg: MessageWithTime) {
  return render(
    <table>
      <tbody>
        <MessageTableRow msg={msg} />
      </tbody>
    </table>
  )
}

function makeMessage(overrides: Partial<MessageWithTime>): MessageWithTime {
  return {
    content: 'hello',
    category: 'text',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('MessageTableRow', () => {
  it('renders a Notice badge and muted styling for system_notice', () => {
    renderRow(makeMessage({ category: 'system_notice', content: 'turn idle' }))

    expect(screen.getByText('Notice')).toBeInTheDocument()
    const row = screen.getByTestId('message-row')
    expect(row.className).not.toContain('border-l-indigo-400')
  })

  it('renders a Task badge with indigo left-accent for task_notification', () => {
    renderRow(makeMessage({ category: 'task_notification', content: 'task done' }))

    expect(screen.getByText('Task')).toBeInTheDocument()
    const row = screen.getByTestId('message-row')
    expect(row.className).toContain('border-l-indigo-400')
    expect(row.className).toContain('bg-indigo-50/30')
  })

  it('still renders the User badge and accent for user_input rows unchanged', () => {
    renderRow(makeMessage({ category: 'user_input', content: 'do the thing' }))

    expect(screen.getByText('User')).toBeInTheDocument()
    const row = screen.getByTestId('message-row')
    expect(row.className).toContain('border-l-primary')
  })

  it('still renders the Error badge and accent for error rows unchanged', () => {
    renderRow(makeMessage({ category: 'error', content: 'boom' }))

    expect(screen.getByText('Error')).toBeInTheDocument()
    const row = screen.getByTestId('message-row')
    expect(row.className).toContain('border-l-destructive')
  })

  it('still renders the Result badge for result rows unchanged', () => {
    renderRow(makeMessage({ category: 'result', content: 'ok' }))

    expect(screen.getByText('Result')).toBeInTheDocument()
  })

  it('still renders italic Thinking styling for thinking rows unchanged', () => {
    renderRow(makeMessage({ category: 'thinking', content: 'pondering' }))

    expect(screen.getByText('Thinking')).toBeInTheDocument()
    expect(screen.getByText('pondering').className).toContain('italic')
  })
})
