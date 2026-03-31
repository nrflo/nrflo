import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FindingsPanel } from './FindingsPanel'
import type { WorkflowFindings } from '@/types/workflow'

function makeAgentFindings(overrides: Partial<WorkflowFindings> = {}): WorkflowFindings {
  return {
    implementor: { result: 'done', notes: 'all tests pass' },
    ...overrides,
  }
}

describe('FindingsPanel', () => {
  describe('empty state', () => {
    it('shows empty message when no findings', () => {
      render(<FindingsPanel projectFindings={undefined} agentFindings={undefined} selectedAgentType={null} />)
      expect(screen.getByText('No findings available')).toBeInTheDocument()
    })

    it('shows empty message when all findings are internal keys only', () => {
      render(
        <FindingsPanel
          projectFindings={{ _orchestration: { phase: 1 } }}
          agentFindings={{ _internal: { foo: 'bar' } }}
          selectedAgentType={null}
        />
      )
      expect(screen.getByText('No findings available')).toBeInTheDocument()
    })
  })

  describe('project findings', () => {
    it('renders Project Findings section header with keys', () => {
      render(
        <FindingsPanel
          projectFindings={{ deploy_url: 'https://example.com', version: '1.2.3' }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      expect(screen.getByText('Project Findings')).toBeInTheDocument()
      expect(screen.getByText('deploy_url')).toBeInTheDocument()
      expect(screen.getByText('version')).toBeInTheDocument()
    })

    it('filters out internal keys starting with _', () => {
      render(
        <FindingsPanel
          projectFindings={{ visible_key: 'value', _hidden_key: 'secret' }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      expect(screen.getByText('visible_key')).toBeInTheDocument()
      expect(screen.queryByText('_hidden_key')).not.toBeInTheDocument()
    })
  })

  describe('agent findings', () => {
    it('renders agent section headers grouped by agent_type', () => {
      render(
        <FindingsPanel
          projectFindings={undefined}
          agentFindings={{
            implementor: { result: 'done' },
            'qa-verifier': { test_count: 42 },
          }}
          selectedAgentType={null}
        />
      )
      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.getByText('qa-verifier')).toBeInTheDocument()
    })

    it('filters out agent _internal keys', () => {
      render(
        <FindingsPanel
          projectFindings={undefined}
          agentFindings={{ implementor: { result: 'pass', _callback: { level: 1 } } }}
          selectedAgentType={null}
        />
      )
      expect(screen.getByText('result')).toBeInTheDocument()
      expect(screen.queryByText('_callback')).not.toBeInTheDocument()
    })

    it('filters to selectedAgentType when set', () => {
      render(
        <FindingsPanel
          projectFindings={undefined}
          agentFindings={{
            implementor: { result: 'done' },
            'qa-verifier': { test_count: 42 },
          }}
          selectedAgentType="implementor"
        />
      )
      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.queryByText('qa-verifier')).not.toBeInTheDocument()
    })

    it('matches agent_type with model suffix (agent_type:model_id)', () => {
      render(
        <FindingsPanel
          projectFindings={undefined}
          agentFindings={{
            'implementor:claude-sonnet-4-5': { result: 'done' },
            'qa-verifier': { test_count: 42 },
          }}
          selectedAgentType="implementor"
        />
      )
      expect(screen.getByText('implementor:claude-sonnet-4-5')).toBeInTheDocument()
      expect(screen.queryByText('qa-verifier')).not.toBeInTheDocument()
    })

    it('skips agent entries with only internal keys', () => {
      render(
        <FindingsPanel
          projectFindings={undefined}
          agentFindings={{
            implementor: { _hidden: 'value' },
            'qa-verifier': { test_count: 42 },
          }}
          selectedAgentType={null}
        />
      )
      expect(screen.queryByText('implementor')).not.toBeInTheDocument()
      expect(screen.getByText('qa-verifier')).toBeInTheDocument()
    })
  })

  describe('value display', () => {
    it('shows short string value inline (no expand needed)', () => {
      render(
        <FindingsPanel
          projectFindings={{ status: 'ok' }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      expect(screen.getByText('status')).toBeInTheDocument()
      // Short values shown inline
      expect(screen.getByText('ok')).toBeInTheDocument()
    })

    it('uses JSON.stringify for object values', async () => {
      const user = userEvent.setup()
      const objValue = { nested: true, count: 3 }
      render(
        <FindingsPanel
          projectFindings={{ config: objValue }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      // Object value is long enough to need expand — click to see it
      const keyButton = screen.getByText('config').closest('button')!
      await user.click(keyButton)
      const pre = document.querySelector('pre')
      expect(pre).toBeInTheDocument()
      expect(pre!.textContent).toContain('"nested": true')
      expect(pre!.textContent).toContain('"count": 3')
    })

    it('parses JSON string values and pretty-prints them', async () => {
      const user = userEvent.setup()
      render(
        <FindingsPanel
          projectFindings={{ data: '{"key":"val","num":1}' }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      const keyButton = screen.getByText('data').closest('button')!
      await user.click(keyButton)
      const pre = document.querySelector('pre')
      expect(pre).toBeInTheDocument()
      expect(pre!.textContent).toContain('"key": "val"')
    })

    it('shows non-JSON string value as plain text when expanded', async () => {
      const user = userEvent.setup()
      const longText = 'a'.repeat(90) // > 80 chars so it needs expand
      render(
        <FindingsPanel
          projectFindings={{ description: longText }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      const keyButton = screen.getByText('description').closest('button')!
      await user.click(keyButton)
      // Should be in a <p> not <pre>
      const p = document.querySelector('p.text-xs.font-mono')
      expect(p).toBeInTheDocument()
      expect(p!.textContent).toBe(longText)
    })
  })

  describe('collapsible toggle', () => {
    it('long values collapsed by default (no pre/p visible)', () => {
      render(
        <FindingsPanel
          projectFindings={{ notes: 'a'.repeat(90) }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      expect(document.querySelector('pre')).not.toBeInTheDocument()
      expect(document.querySelector('p.text-xs.font-mono')).not.toBeInTheDocument()
    })

    it('clicking expand button shows content', async () => {
      const user = userEvent.setup()
      render(
        <FindingsPanel
          projectFindings={{ notes: 'a'.repeat(90) }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      const keyButton = screen.getByText('notes').closest('button')!
      await user.click(keyButton)
      expect(document.querySelector('p.text-xs.font-mono')).toBeInTheDocument()
    })

    it('clicking again collapses back', async () => {
      const user = userEvent.setup()
      render(
        <FindingsPanel
          projectFindings={{ notes: 'a'.repeat(90) }}
          agentFindings={undefined}
          selectedAgentType={null}
        />
      )
      const keyButton = screen.getByText('notes').closest('button')!
      await user.click(keyButton)
      expect(document.querySelector('p.text-xs.font-mono')).toBeInTheDocument()
      await user.click(keyButton)
      expect(document.querySelector('p.text-xs.font-mono')).not.toBeInTheDocument()
    })
  })

  describe('section ordering', () => {
    it('renders Project Findings before Agent Findings', () => {
      render(
        <FindingsPanel
          projectFindings={{ pf_key: 'pf_val' }}
          agentFindings={makeAgentFindings()}
          selectedAgentType={null}
        />
      )
      const headers = screen.getAllByRole('heading', { level: 4 })
      expect(headers[0].textContent).toBe('Project Findings')
      // Agent section header follows
      expect(headers[1].textContent).toBe('implementor')
    })
  })
})
