import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LogMessage } from './LogMessage'

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
})
