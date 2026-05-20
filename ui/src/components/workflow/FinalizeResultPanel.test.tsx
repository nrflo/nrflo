import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FinalizeResultPanel } from './FinalizeResultPanel'
import type { FinalizeResult } from '@/types/workflow'

function makeResult(overrides: Partial<FinalizeResult> = {}): FinalizeResult {
  return {
    slot: 'success',
    kind: 'command',
    target: 'make deploy',
    exit_code: 0,
    status: 'ok',
    ...overrides,
  }
}

describe('FinalizeResultPanel', () => {
  it('returns null when result is undefined', () => {
    const { container } = render(<FinalizeResultPanel result={undefined} />)
    expect(container.firstChild).toBeNull()
  })

  describe('success shape (status: ok)', () => {
    it('shows green styling for ok status', () => {
      const { container } = render(<FinalizeResultPanel result={makeResult({ status: 'ok' })} />)
      expect((container.firstChild as HTMLElement).className).toMatch(/green/)
    })

    it('displays slot and command target', () => {
      render(<FinalizeResultPanel result={makeResult({ slot: 'success', kind: 'command', target: 'make deploy' })} />)
      expect(screen.getByText(/finalize \(success\)/i)).toBeInTheDocument()
      expect(screen.getByText(/make deploy/)).toBeInTheDocument()
    })

    it('shows script: prefix for kind=script', () => {
      render(<FinalizeResultPanel result={makeResult({ kind: 'script', target: 'my-script-id' })} />)
      expect(screen.getByText(/script:my-script-id/)).toBeInTheDocument()
    })

    it('shows exit code and status', () => {
      render(<FinalizeResultPanel result={makeResult({ exit_code: 0, status: 'ok' })} />)
      expect(screen.getByText(/exit 0/i)).toBeInTheDocument()
      expect(screen.getByText(/ok/)).toBeInTheDocument()
    })

    it('does not render output_tail when absent', () => {
      render(<FinalizeResultPanel result={makeResult({ output_tail: undefined })} />)
      expect(screen.queryByRole('code')).toBeNull()
    })
  })

  describe('failure shape (status: failed)', () => {
    it('shows red styling for failed status', () => {
      const { container } = render(<FinalizeResultPanel result={makeResult({ status: 'failed', slot: 'failure' })} />)
      expect((container.firstChild as HTMLElement).className).toMatch(/red/)
    })

    it('displays failure slot', () => {
      render(<FinalizeResultPanel result={makeResult({ slot: 'failure', status: 'failed' })} />)
      expect(screen.getByText(/finalize \(failure\)/i)).toBeInTheDocument()
    })

    it('renders output_tail in pre element when present', () => {
      render(<FinalizeResultPanel result={makeResult({ status: 'failed', output_tail: 'Error: command not found' })} />)
      expect(screen.getByText('Error: command not found')).toBeInTheDocument()
      const pre = screen.getByText('Error: command not found').closest('pre')
      expect(pre).toBeInTheDocument()
    })

    it('shows exit code for failed status', () => {
      render(<FinalizeResultPanel result={makeResult({ status: 'failed', exit_code: 1 })} />)
      expect(screen.getByText(/exit 1/i)).toBeInTheDocument()
    })
  })

  describe('timeout shape', () => {
    it('shows red styling for timeout status', () => {
      const { container } = render(<FinalizeResultPanel result={makeResult({ status: 'timeout' })} />)
      expect((container.firstChild as HTMLElement).className).toMatch(/red/)
    })

    it('shows timeout in status text', () => {
      render(<FinalizeResultPanel result={makeResult({ status: 'timeout', exit_code: 124 })} />)
      expect(screen.getByText(/timeout/)).toBeInTheDocument()
    })
  })
})
