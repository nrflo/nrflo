import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ChatMessageList } from './ChatMessageList'
import type { ConsoleContextRotatedPayload } from '@/types/consoleChat'
import type { MessageWithTime } from '@/types/workflow'

const baseProps = {
  sid: 's1',
  transcript: [],
  approvals: [],
  resolvedApprovals: new Map(),
  liveThinking: [],
  turn: 'idle' as const,
}

function messageItem(message: MessageWithTime) {
  return { kind: 'message' as const, message }
}

describe('ChatMessageList rotation divider', () => {
  it('renders a divider per rotation notice, in order, with formatted token counts', () => {
    const rotations: ConsoleContextRotatedPayload[] = [
      { session_id: 's1', tokens_before: 9000, tokens_after: 1200 },
      { session_id: 's1', tokens_before: 8500, tokens_after: 1100 },
    ]
    render(<ChatMessageList {...baseProps} rotations={rotations} />)

    expect(screen.getByText('context rotated · 9,000 → 1,200 tokens')).toBeInTheDocument()
    expect(screen.getByText('context rotated · 8,500 → 1,100 tokens')).toBeInTheDocument()
  })

  it('renders no divider when there are no rotations', () => {
    render(<ChatMessageList {...baseProps} rotations={[]} />)
    expect(screen.queryByText(/context rotated/)).not.toBeInTheDocument()
  })
})

describe('ChatMessageList category rendering', () => {
  it('renders nothing for a system_notice message', () => {
    const notice: MessageWithTime = {
      content: 'turn idle',
      category: 'system_notice',
      created_at: '2026-01-01T00:00:00Z',
    }
    render(<ChatMessageList {...baseProps} transcript={[messageItem(notice)]} rotations={[]} />)

    expect(screen.queryByText('turn idle')).not.toBeInTheDocument()
  })

  it('renders a task_notification message as a ChatTaskNotification card', () => {
    const notification: MessageWithTime = {
      content: '<task-notification><task-id>t1</task-id><status>completed</status><summary>done</summary></task-notification>',
      category: 'task_notification',
      created_at: '2026-01-01T00:00:00Z',
    }
    render(<ChatMessageList {...baseProps} transcript={[messageItem(notification)]} rotations={[]} />)

    expect(screen.getByText('Task')).toBeInTheDocument()
    expect(screen.getByText(/task t1 · completed — done/)).toBeInTheDocument()
  })
})
