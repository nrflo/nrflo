import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChatTaskNotification } from './ChatTaskNotification'

describe('ChatTaskNotification', () => {
  it('renders a collapsed summary line with task id, status, and summary', () => {
    const content = [
      '<task-notification>',
      '<task-id>bdt1d3p3q</task-id>',
      '<status>completed</status>',
      '<summary>Background command finished</summary>',
      '<result>exit code 0</result>',
      '</task-notification>',
    ].join('\n')

    render(<ChatTaskNotification content={content} />)

    expect(screen.getByText('Task')).toBeInTheDocument()
    expect(screen.getByText(/task bdt1d3p3q · completed — Background command finished/)).toBeInTheDocument()
    // Result body is present in the DOM (inside <details>) but not visible until expanded.
    expect(screen.getByText('exit code 0')).toBeInTheDocument()
  })

  it('expands to reveal the result body on click', async () => {
    const user = userEvent.setup()
    const content = '<task-notification><task-id>t1</task-id><status>completed</status><result>the answer</result></task-notification>'
    render(<ChatTaskNotification content={content} />)

    const summary = screen.getByText(/task t1 · completed/)
    const details = summary.closest('details')!
    expect(details.open).toBe(false)

    await user.click(summary)
    expect(details.open).toBe(true)
    expect(screen.getByText('the answer')).toBeInTheDocument()
  })

  it('renders taskId/status fallback text when tags are missing', () => {
    render(<ChatTaskNotification content="<task-notification></task-notification>" />)
    expect(screen.getByText(/task unknown · unknown/)).toBeInTheDocument()
  })

  it('falls back to a raw content box when content is not a task-notification envelope', () => {
    render(<ChatTaskNotification content="plain unparsable text" />)
    expect(screen.getByText('plain unparsable text')).toBeInTheDocument()
    expect(screen.queryByText('Task')).not.toBeInTheDocument()
  })
})
