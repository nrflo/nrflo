import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LogMessage, parseToolName, ToolBadge } from './LogMessage'

describe('LogMessage', () => {
  describe('compact variant (default)', () => {
    it('renders message text', () => {
      render(<LogMessage message="some message text" />)
      expect(screen.getByText('some message text')).toBeInTheDocument()
    })

    it('renders with whitespace-pre-wrap for text wrapping', () => {
      render(<LogMessage message="A long message" />)
      const el = screen.getByText('A long message')
      expect(el.className).toContain('whitespace-pre-wrap')
    })

    it('applies font-mono and text-xs styling', () => {
      render(<LogMessage message="test" />)
      const el = screen.getByText('test')
      expect(el.className).toContain('font-mono')
      expect(el.className).toContain('text-xs')
    })

    it('merges custom className', () => {
      render(<LogMessage message="test" className="my-custom-class" />)
      const el = screen.getByText('test')
      expect(el.className).toContain('my-custom-class')
    })
  })

  describe('full variant', () => {
    it('renders message text', () => {
      render(<LogMessage message="Full log message" variant="full" />)
      expect(screen.getByText('Full log message')).toBeInTheDocument()
    })

    it('renders with whitespace-pre-wrap for multiline content', () => {
      render(<LogMessage message={'line1\nline2'} variant="full" />)
      const el = screen.getByText((_content, element) =>
        element?.textContent === 'line1\nline2' && element?.className?.includes('whitespace-pre-wrap') || false
      )
      expect(el.className).toContain('whitespace-pre-wrap')
    })

    it('does not truncate text', () => {
      render(<LogMessage message="Full message" variant="full" />)
      const el = screen.getByText('Full message')
      expect(el.className).not.toContain('truncate')
    })

    it('applies font-mono and text-sm styling', () => {
      render(<LogMessage message="test" variant="full" />)
      const el = screen.getByText('test')
      expect(el.className).toContain('font-mono')
      expect(el.className).toContain('text-sm')
    })

    it('merges custom className', () => {
      render(<LogMessage message="test" variant="full" className="extra-class" />)
      const el = screen.getByText('test')
      expect(el.className).toContain('extra-class')
    })
  })

  describe('tool name highlighting', () => {
    it('renders tool badge for [Read] prefix', () => {
      render(<LogMessage message="[Read] src/main.ts" />)
      expect(screen.getByText('Read')).toBeInTheDocument()
      expect(screen.getByText('src/main.ts')).toBeInTheDocument()
    })

    it('renders tool badge for [Edit] prefix', () => {
      render(<LogMessage message="[Edit] src/utils.ts" />)
      expect(screen.getByText('Edit')).toBeInTheDocument()
      expect(screen.getByText('src/utils.ts')).toBeInTheDocument()
    })

    it('renders tool badge for [Bash] prefix', () => {
      render(<LogMessage message="[Bash] npm install" />)
      expect(screen.getByText('Bash')).toBeInTheDocument()
      expect(screen.getByText('npm install')).toBeInTheDocument()
    })

    it('does not render badge for messages without tool prefix', () => {
      render(<LogMessage message="plain message" />)
      expect(screen.getByText('plain message')).toBeInTheDocument()
      expect(screen.queryByText('Read')).not.toBeInTheDocument()
    })
  })

  describe('shared styling', () => {
    it('both variants have border and bg-muted/30', () => {
      const { rerender } = render(<LogMessage message="compact" />)
      const compact = screen.getByText('compact')
      expect(compact.className).toContain('border')
      expect(compact.className).toContain('bg-muted/30')

      rerender(<LogMessage message="full" variant="full" />)
      const full = screen.getByText('full')
      expect(full.className).toContain('border')
      expect(full.className).toContain('bg-muted/30')
    })
  })

  describe('no tooltip rendering', () => {
    it('does not render any tooltip elements on hover', () => {
      render(<LogMessage message="[Bash] git status" />)
      // No tooltip-related attributes or elements
      const el = screen.getByText('git status').closest('div')!
      expect(el.querySelector('[role="tooltip"]')).toBeNull()
      expect(el.getAttribute('title')).toBeNull()
      expect(el.getAttribute('data-tooltip')).toBeNull()
    })
  })
})

describe('parseToolName', () => {
  it('extracts tool name and rest from bracketed prefix', () => {
    expect(parseToolName('[Bash] git status')).toEqual({ toolName: 'Bash', rest: 'git status' })
  })

  it('extracts tool name for Read', () => {
    expect(parseToolName('[Read] /path/to/file.ts')).toEqual({ toolName: 'Read', rest: '/path/to/file.ts' })
  })

  it('extracts tool name for multiline rest', () => {
    const result = parseToolName('[Edit] first line\nsecond line')
    expect(result.toolName).toBe('Edit')
    expect(result.rest).toBe('first line\nsecond line')
  })

  it('returns null toolName for plain messages', () => {
    expect(parseToolName('plain message')).toEqual({ toolName: null, rest: 'plain message' })
  })

  it('returns null toolName and empty rest for empty string', () => {
    expect(parseToolName('')).toEqual({ toolName: null, rest: '' })
  })

  it('does not match brackets in the middle of the string', () => {
    expect(parseToolName('some [Bash] in middle')).toEqual({ toolName: null, rest: 'some [Bash] in middle' })
  })
})

describe('ToolBadge', () => {
  it('renders badge with known tool color', () => {
    render(<ToolBadge name="Bash" />)
    const badge = screen.getByText('Bash')
    expect(badge).toBeInTheDocument()
    expect(badge.className).toContain('bg-blue-100')
  })

  it('renders badge with default color for unknown tool', () => {
    render(<ToolBadge name="UnknownTool" />)
    const badge = screen.getByText('UnknownTool')
    expect(badge).toBeInTheDocument()
    expect(badge.className).toContain('bg-gray-100')
  })

  it('renders TaskResult badge with emerald-100 styling', () => {
    render(<ToolBadge name="TaskResult" />)
    const badge = screen.getByText('TaskResult')
    expect(badge).toBeInTheDocument()
    expect(badge.className).toContain('bg-emerald-100')
  })

  it('renders Task badge with indigo-100 styling', () => {
    render(<ToolBadge name="Task" />)
    const badge = screen.getByText('Task')
    expect(badge).toBeInTheDocument()
    expect(badge.className).toContain('bg-indigo-100')
  })

  it('TaskResult uses emerald (green) while Task uses indigo', () => {
    const { rerender } = render(<ToolBadge name="Task" />)
    const taskBadge = screen.getByText('Task')
    expect(taskBadge.className).toContain('bg-indigo-100')
    expect(taskBadge.className).not.toContain('bg-emerald-100')

    rerender(<ToolBadge name="TaskResult" />)
    const resultBadge = screen.getByText('TaskResult')
    expect(resultBadge.className).toContain('bg-emerald-100')
    expect(resultBadge.className).not.toContain('bg-indigo-100')
  })
})
