import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LogMessage, ToolBadge } from './LogMessage'

describe('LogMessage — consult tool-use rendering', () => {
  it('renders "consulted <consultant> — <question>" for JSON tool-use row', () => {
    const json = JSON.stringify({ consultant: 'sec-expert', question: 'Is X safe?' })
    render(<LogMessage message={`[consult] ${json}`} category="tool" />)
    expect(screen.getByText('consult')).toBeInTheDocument()
    expect(screen.getByText('consulted sec-expert — Is X safe?')).toBeInTheDocument()
  })

  it('renders only consultant part when question is absent', () => {
    const json = JSON.stringify({ consultant: 'arch-advisor' })
    render(<LogMessage message={`[consult] ${json}`} category="tool" />)
    expect(screen.getByText('consulted arch-advisor')).toBeInTheDocument()
  })

  it('renders only question part when consultant is absent', () => {
    const json = JSON.stringify({ question: 'What is the answer?' })
    render(<LogMessage message={`[consult] ${json}`} category="tool" />)
    expect(screen.getByText('What is the answer?')).toBeInTheDocument()
  })

  it('renders tool-result answer after "→ " prefix', () => {
    render(<LogMessage message="[consult] → The answer is 42." category="tool_result" />)
    expect(screen.getByText('consult')).toBeInTheDocument()
    expect(screen.getByText('The answer is 42.')).toBeInTheDocument()
  })

  it('renders plain tool-result answer without arrow prefix', () => {
    render(<LogMessage message="[consult] plain answer text" category="tool_result" />)
    expect(screen.getByText('plain answer text')).toBeInTheDocument()
  })

  it('falls back to raw text when JSON is malformed', () => {
    render(<LogMessage message="[consult] {not valid json}" category="tool" />)
    expect(screen.getByText('{not valid json}')).toBeInTheDocument()
  })
})

describe('LogMessage — consult indigo accent styling', () => {
  it('applies indigo left-border accent for consult messages', () => {
    const json = JSON.stringify({ consultant: 'advisor', question: 'Check this?' })
    render(<LogMessage message={`[consult] ${json}`} />)
    const container = screen.getByText('consult').closest('div')!
    expect(container.className).toContain('border-l-4')
    expect(container.className).toContain('border-l-indigo-400')
    expect(container.className).toContain('bg-indigo-50/30')
  })

  it('non-consult tool messages do not get indigo accent', () => {
    render(<LogMessage message="[Bash] git status" />)
    const container = screen.getByText('git status').closest('div')!
    expect(container.className).not.toContain('border-l-indigo-400')
    expect(container.className).not.toContain('bg-indigo-50/30')
  })
})

describe('ToolBadge — consult color', () => {
  it('renders consult badge with indigo styling', () => {
    render(<ToolBadge name="consult" />)
    const badge = screen.getByText('consult')
    expect(badge.className).toContain('bg-indigo-100')
    expect(badge.className).toContain('text-indigo-800')
  })
})
