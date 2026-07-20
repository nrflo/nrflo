import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import { ChatToolCard } from './ChatToolCard'
import type { ToolPair } from './chatStream'
import type { MessageWithTime } from '@/types/workflow'

function msg(overrides: Partial<MessageWithTime> = {}): MessageWithTime {
  return { content: '[Bash] ls -la', category: 'tool', created_at: '2026-01-01T00:00:00Z', ...overrides }
}

function pair(overrides: Partial<ToolPair> = {}): ToolPair {
  return {
    invoke: msg(),
    invokeIndex: 0,
    toolUseId: 't1',
    running: false,
    ...overrides,
  }
}

describe('ChatToolCard', () => {
  it('renders a paired invoke+result with the tool badge, result detail, and duration chip', () => {
    renderWithQuery(
      <ChatToolCard
        pair={pair({
          result: msg({ content: '[Bash] file1\nfile2' }),
          durationMs: 2500,
        })}
      />
    )

    expect(screen.getByText('Bash')).toBeInTheDocument()
    expect(screen.getByText('ls -la')).toBeInTheDocument()
    expect(screen.getByText('2.5s')).toBeInTheDocument()
    expect(screen.getByText('result')).toBeInTheDocument()
    expect(screen.getByText((_, el) => el?.textContent === 'file1\nfile2')).toBeInTheDocument()
  })

  it('formats a sub-second duration in milliseconds', () => {
    renderWithQuery(<ChatToolCard pair={pair({ result: msg(), durationMs: 250 })} />)
    expect(screen.getByText('250ms')).toBeInTheDocument()
  })

  it('renders as still-running when there is no ended_at and no result', () => {
    renderWithQuery(<ChatToolCard pair={pair({ running: true })} />)

    expect(screen.getByText('running…')).toBeInTheDocument()
    expect(screen.queryByText('result')).not.toBeInTheDocument()
  })

  it('renders destructive styling for an error-category result', () => {
    renderWithQuery(
      <ChatToolCard
        pair={pair({
          result: msg({ category: 'error', content: '[Bash] command not found' }),
          durationMs: 100,
        })}
      />
    )

    const card = screen.getByTestId('chat-tool-card')
    expect(card.className).toContain('border-l-destructive')
    expect(screen.getByText('command not found')).toBeInTheDocument()
  })

  it('renders a collapsible input section when the pair carries structured input', () => {
    renderWithQuery(<ChatToolCard pair={pair({ input: { command: 'ls -la' } })} />)

    expect(screen.getByText('input')).toBeInTheDocument()
    expect(screen.getByText((_, el) => el?.textContent === '{\n  "command": "ls -la"\n}')).toBeInTheDocument()
    expect(screen.queryByText('(truncated)')).not.toBeInTheDocument()
  })

  it('marks the input section as truncated when inputTruncated is set', () => {
    renderWithQuery(<ChatToolCard pair={pair({ input: { command: 'ls' }, inputTruncated: true })} />)

    expect(screen.getByText('(truncated)')).toBeInTheDocument()
  })

  it('renders a truncated placeholder when input was omitted for exceeding the size cap', () => {
    renderWithQuery(<ChatToolCard pair={pair({ inputTruncated: true })} />)

    expect(screen.getByText('input')).toBeInTheDocument()
    expect(screen.getByText('(truncated)')).toBeInTheDocument()
    expect(screen.getByText('(input too large to display)')).toBeInTheDocument()
  })

  it('renders no input section and falls back to the formatted content for older rows without input', () => {
    renderWithQuery(<ChatToolCard pair={pair()} />)

    expect(screen.queryByText('input')).not.toBeInTheDocument()
    expect(screen.getByText('ls -la')).toBeInTheDocument()
  })
})
