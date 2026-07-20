import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ChatStatusBar } from './ChatStatusBar'

describe('ChatStatusBar', () => {
  it('renders engine·model, profile, workDir, context-left, cost, and Running… when all are set', () => {
    render(
      <ChatStatusBar
        engine="claude"
        model="sonnet"
        profile="t0-bare"
        workDir="/tmp/w"
        contextLeft={42}
        cost={1.234}
        turn="running"
      />
    )

    expect(screen.getByText('claude')).toBeInTheDocument()
    expect(screen.getByText('· sonnet')).toBeInTheDocument()
    expect(screen.getByText('t0-bare')).toBeInTheDocument()
    expect(screen.getByText('/tmp/w')).toBeInTheDocument()
    expect(screen.getByText('Context left: 42%')).toBeInTheDocument()
    expect(screen.getByText('~$1.23')).toBeInTheDocument()
    expect(screen.getByText('Running…')).toBeInTheDocument()
  })

  it('omits profile, workDir, context-left, and cost when unset; shows Idle', () => {
    render(<ChatStatusBar engine="claude" model="sonnet" turn="idle" />)

    expect(screen.getByText('claude')).toBeInTheDocument()
    expect(screen.queryByText(/Context left:/)).not.toBeInTheDocument()
    expect(screen.queryByText(/^~\$/)).not.toBeInTheDocument()
    expect(screen.getByText('Idle')).toBeInTheDocument()
  })

  it('omits the model separator when model is unset', () => {
    render(<ChatStatusBar engine="claude" turn="idle" />)
    expect(screen.queryByText(/·/)).not.toBeInTheDocument()
  })
})
