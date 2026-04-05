import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AllFindingsPanel } from './AllFindingsPanel'
import type { WorkflowFindings } from '@/types/workflow'

function makeAgentFindings(overrides: Partial<WorkflowFindings> = {}): WorkflowFindings {
  return {
    implementor: { result: 'done' },
    ...overrides,
  }
}

describe('AllFindingsPanel', () => {
  describe('empty state', () => {
    it('shows empty message when no findings provided', () => {
      render(<AllFindingsPanel />)
      expect(screen.getByText('No findings available')).toBeInTheDocument()
    })

    it('shows empty message when all findings are internal keys', () => {
      render(
        <AllFindingsPanel
          workflowFindings={{ _orchestration: { phase: 1 } }}
          agentFindings={{ _internal: { foo: 'bar' } }}
          projectFindings={{ _hidden: 'val' }}
        />
      )
      expect(screen.getByText('No findings available')).toBeInTheDocument()
    })

    it('shows empty message when agent findings only have internal keys', () => {
      render(
        <AllFindingsPanel
          agentFindings={{ implementor: { _callback: { level: 1 } } }}
        />
      )
      expect(screen.getByText('No findings available')).toBeInTheDocument()
    })
  })

  describe('workflow findings section', () => {
    it('renders Workflow Findings header with non-internal keys', () => {
      render(
        <AllFindingsPanel
          workflowFindings={{ final_result: 'success', status: 'done' }}
        />
      )
      expect(screen.getByText('Workflow Findings')).toBeInTheDocument()
      expect(screen.getByText('final_result')).toBeInTheDocument()
      expect(screen.getByText('status')).toBeInTheDocument()
    })

    it('hides internal keys in workflow findings', () => {
      render(
        <AllFindingsPanel
          workflowFindings={{ visible_key: 'val', _orchestration: { phase: 1 } }}
        />
      )
      expect(screen.getByText('visible_key')).toBeInTheDocument()
      expect(screen.queryByText('_orchestration')).not.toBeInTheDocument()
    })

    it('skips Workflow Findings section when all keys are internal', () => {
      render(
        <AllFindingsPanel
          workflowFindings={{ _callback: { level: 1 } }}
          agentFindings={{ implementor: { result: 'done' } }}
        />
      )
      expect(screen.queryByText('Workflow Findings')).not.toBeInTheDocument()
    })
  })

  describe('project findings section', () => {
    it('renders Project Findings header with keys', () => {
      render(
        <AllFindingsPanel
          projectFindings={{ deploy_url: 'https://example.com', version: '1.2.3' }}
        />
      )
      expect(screen.getByText('Project Findings')).toBeInTheDocument()
      expect(screen.getByText('deploy_url')).toBeInTheDocument()
      expect(screen.getByText('version')).toBeInTheDocument()
    })

    it('filters out internal keys in project findings', () => {
      render(
        <AllFindingsPanel
          projectFindings={{ visible: 'val', _hidden: 'secret' }}
        />
      )
      expect(screen.getByText('visible')).toBeInTheDocument()
      expect(screen.queryByText('_hidden')).not.toBeInTheDocument()
    })

    it('skips Project Findings section when empty', () => {
      render(
        <AllFindingsPanel
          agentFindings={makeAgentFindings()}
        />
      )
      expect(screen.queryByText('Project Findings')).not.toBeInTheDocument()
    })
  })

  describe('agent findings section', () => {
    it('renders agent section headers with findings', () => {
      render(
        <AllFindingsPanel
          agentFindings={{
            implementor: { result: 'done' },
            'qa-verifier': { test_count: 42 },
          }}
        />
      )
      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.getByText('qa-verifier')).toBeInTheDocument()
    })

    it('filters out internal keys within agent findings', () => {
      render(
        <AllFindingsPanel
          agentFindings={{ implementor: { result: 'pass', _callback: { level: 1 } } }}
        />
      )
      expect(screen.getByText('result')).toBeInTheDocument()
      expect(screen.queryByText('_callback')).not.toBeInTheDocument()
    })

    it('skips agent entries with only internal keys', () => {
      render(
        <AllFindingsPanel
          agentFindings={{
            implementor: { _hidden: 'val' },
            'qa-verifier': { test_count: 5 },
          }}
        />
      )
      expect(screen.queryByText('implementor')).not.toBeInTheDocument()
      expect(screen.getByText('qa-verifier')).toBeInTheDocument()
    })
  })

  describe('layer sorting', () => {
    it('sorts agents ascending by layer number', () => {
      render(
        <AllFindingsPanel
          agentFindings={{
            'qa-verifier': { tests: 10 },
            implementor: { result: 'done' },
            'setup-analyzer': { analysis: 'ok' },
          }}
          phaseLayers={{ 'setup-analyzer': 0, implementor: 2, 'qa-verifier': 3 }}
        />
      )
      const headers = screen.getAllByRole('heading', { level: 4 })
      const texts = headers.map(h => h.textContent)
      const setupIdx = texts.findIndex(t => t?.includes('setup-analyzer'))
      const implIdx = texts.findIndex(t => t?.includes('implementor'))
      const qaIdx = texts.findIndex(t => t?.includes('qa-verifier'))
      expect(setupIdx).toBeLessThan(implIdx)
      expect(implIdx).toBeLessThan(qaIdx)
    })

    it('shows layer label in section header (L0)', () => {
      render(
        <AllFindingsPanel
          agentFindings={{ implementor: { result: 'done' } }}
          phaseLayers={{ implementor: 0 }}
        />
      )
      expect(screen.getByText('implementor (L0)')).toBeInTheDocument()
    })

    it('shows correct layer label for higher layers', () => {
      render(
        <AllFindingsPanel
          agentFindings={{ 'qa-verifier': { tests: 5 } }}
          phaseLayers={{ 'qa-verifier': 3 }}
        />
      )
      expect(screen.getByText('qa-verifier (L3)')).toBeInTheDocument()
    })

    it('agents without layer mapping sort last (no label suffix)', () => {
      render(
        <AllFindingsPanel
          agentFindings={{
            unknown: { data: 'x' },
            implementor: { result: 'done' },
          }}
          phaseLayers={{ implementor: 0 }}
        />
      )
      const headers = screen.getAllByRole('heading', { level: 4 })
      const texts = headers.map(h => h.textContent)
      const implIdx = texts.findIndex(t => t?.includes('implementor'))
      const unknownIdx = texts.findIndex(t => t?.includes('unknown'))
      expect(implIdx).toBeLessThan(unknownIdx)
      // No layer label on unknown
      expect(screen.queryByText('unknown (L')).not.toBeInTheDocument()
      expect(screen.getByText('unknown')).toBeInTheDocument()
    })

    it('strips model suffix when looking up layer (implementor:claude-sonnet-4-5)', () => {
      render(
        <AllFindingsPanel
          agentFindings={{ 'implementor:claude-sonnet-4-5': { result: 'done' } }}
          phaseLayers={{ implementor: 2 }}
        />
      )
      // Should show "implementor:claude-sonnet-4-5 (L2)" — layer resolved via stripped key
      expect(screen.getByText('implementor:claude-sonnet-4-5 (L2)')).toBeInTheDocument()
    })

    it('handles undefined phaseLayers gracefully', () => {
      render(
        <AllFindingsPanel
          agentFindings={{ implementor: { result: 'done' } }}
          phaseLayers={undefined}
        />
      )
      // No layer label suffix when phaseLayers is undefined
      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.queryByText('implementor (L')).not.toBeInTheDocument()
    })
  })

  describe('section ordering', () => {
    it('renders Workflow Findings before Project Findings before Agent Findings', () => {
      render(
        <AllFindingsPanel
          workflowFindings={{ wf_key: 'wf_val' }}
          projectFindings={{ pf_key: 'pf_val' }}
          agentFindings={{ implementor: { result: 'done' } }}
          phaseLayers={{ implementor: 0 }}
        />
      )
      const headers = screen.getAllByRole('heading', { level: 4 })
      const texts = headers.map(h => h.textContent)
      expect(texts[0]).toBe('Workflow Findings')
      expect(texts[1]).toBe('Project Findings')
      expect(texts[2]).toBe('implementor (L0)')
    })
  })

  describe('string value regression (char-per-line bug)', () => {
    it('does not render character indices when agent findings entry is a plain string', () => {
      // Simulates a buggy backend response where a workflow-level string value (e.g.
      // user_instructions) leaks into agentFindings. Object.keys() on a string returns
      // character indices ["0","1","2",...] which would render one char per row.
      render(
        <AllFindingsPanel
          agentFindings={{
            user_instructions: 'follow these rules' as unknown as Record<string, unknown>,
          }}
        />
      )
      expect(screen.queryByText('0')).not.toBeInTheDocument()
      expect(screen.queryByText('1')).not.toBeInTheDocument()
      expect(screen.queryByText('2')).not.toBeInTheDocument()
      // The agent section itself should be suppressed (no valid object keys)
      expect(screen.queryByText('user_instructions')).not.toBeInTheDocument()
      expect(screen.getByText('No findings available')).toBeInTheDocument()
    })

    it('still renders agent sections when other agents have valid object findings', () => {
      render(
        <AllFindingsPanel
          agentFindings={{
            user_instructions: 'some text' as unknown as Record<string, unknown>,
            implementor: { result: 'pass' },
          }}
        />
      )
      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.getByText('result')).toBeInTheDocument()
      expect(screen.queryByText('0')).not.toBeInTheDocument()
      expect(screen.queryByText('user_instructions')).not.toBeInTheDocument()
    })
  })

  describe('FindingRow integration', () => {
    it('expands a long finding value on click', async () => {
      const user = userEvent.setup()
      const longText = 'x'.repeat(90)
      render(
        <AllFindingsPanel
          workflowFindings={{ notes: longText }}
        />
      )
      const btn = screen.getByText('notes').closest('button')!
      await user.click(btn)
      expect(document.querySelector('p.text-xs.font-mono')).toBeInTheDocument()
    })
  })
})
