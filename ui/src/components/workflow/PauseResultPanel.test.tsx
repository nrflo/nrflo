import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PauseResultPanel } from './PauseResultPanel'
import type { PauseResult } from '@/types/workflow'

function makeResult(overrides: Partial<PauseResult> = {}): PauseResult {
  return {
    paused_after_layer: 0,
    resume_layer: 1,
    ...overrides,
  }
}

describe('PauseResultPanel', () => {
  it('returns null when result is undefined', () => {
    const { container } = render(<PauseResultPanel result={undefined} />)
    expect(container.firstChild).toBeNull()
  })

  it('shows layer info', () => {
    render(<PauseResultPanel result={makeResult({ paused_after_layer: 2, resume_layer: 3 })} />)
    expect(screen.getByText(/paused after layer 2/i)).toBeInTheDocument()
    expect(screen.getByText(/resume at layer 3/i)).toBeInTheDocument()
  })

  it('uses yellow styling when no event (no hook ran)', () => {
    const { container } = render(<PauseResultPanel result={makeResult()} />)
    expect((container.firstChild as HTMLElement).className).toMatch(/yellow/)
  })

  it('uses yellow styling when event status is ok', () => {
    const result = makeResult({
      event: { kind: 'command', target: 'make pause', exit_code: 0, status: 'ok' },
    })
    const { container } = render(<PauseResultPanel result={result} />)
    expect((container.firstChild as HTMLElement).className).toMatch(/yellow/)
  })

  it('uses red styling when event status is failed', () => {
    const result = makeResult({
      event: { kind: 'command', target: 'make pause', exit_code: 1, status: 'failed' },
    })
    const { container } = render(<PauseResultPanel result={result} />)
    expect((container.firstChild as HTMLElement).className).toMatch(/red/)
  })

  it('uses red styling when event status is timeout', () => {
    const result = makeResult({
      event: { kind: 'command', target: 'make pause', exit_code: 124, status: 'timeout' },
    })
    const { container } = render(<PauseResultPanel result={result} />)
    expect((container.firstChild as HTMLElement).className).toMatch(/red/)
  })

  describe('with event (hook ran)', () => {
    it('shows command target', () => {
      render(<PauseResultPanel result={makeResult({
        event: { kind: 'command', target: 'make notify', exit_code: 0, status: 'ok' },
      })} />)
      expect(screen.getByText(/make notify/)).toBeInTheDocument()
    })

    it('shows script: prefix for script kind', () => {
      render(<PauseResultPanel result={makeResult({
        event: { kind: 'script', target: 'my-script-id', exit_code: 0, status: 'ok' },
      })} />)
      expect(screen.getByText(/script:my-script-id/)).toBeInTheDocument()
    })

    it('shows exit code', () => {
      render(<PauseResultPanel result={makeResult({
        event: { kind: 'command', target: 'cmd', exit_code: 1, status: 'failed' },
      })} />)
      expect(screen.getByText(/exit 1/i)).toBeInTheDocument()
    })

    it('shows status text', () => {
      render(<PauseResultPanel result={makeResult({
        event: { kind: 'command', target: 'cmd', exit_code: 0, status: 'ok' },
      })} />)
      expect(screen.getByText(/\bok\b/)).toBeInTheDocument()
    })

    it('renders output_tail in pre element when present', () => {
      render(<PauseResultPanel result={makeResult({
        event: { kind: 'command', target: 'cmd', exit_code: 1, status: 'failed', output_tail: 'Error: failed to connect' },
      })} />)
      expect(screen.getByText('Error: failed to connect')).toBeInTheDocument()
      const pre = screen.getByText('Error: failed to connect').closest('pre')
      expect(pre).toBeInTheDocument()
    })

    it('does not render output_tail section when absent', () => {
      render(<PauseResultPanel result={makeResult({
        event: { kind: 'command', target: 'cmd', exit_code: 0, status: 'ok' },
      })} />)
      expect(screen.queryByRole('code')).toBeNull()
    })
  })

  describe('without event', () => {
    it('does not show hook line when no event', () => {
      render(<PauseResultPanel result={makeResult()} />)
      expect(screen.queryByText(/hook:/i)).not.toBeInTheDocument()
    })
  })
})
