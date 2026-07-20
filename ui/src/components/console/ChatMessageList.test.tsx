import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ChatMessageList } from './ChatMessageList'
import type { ConsoleContextRotatedPayload } from '@/types/consoleChat'

const baseProps = {
  sid: 's1',
  transcript: [],
  approvals: [],
  resolvedApprovals: new Map(),
  liveThinking: [],
  turn: 'idle' as const,
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
