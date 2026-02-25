import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'
import type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest } from '@/types/workflow'

function makeAgentDef(overrides: Partial<AgentDef> = {}): AgentDef {
  return {
    id: 'test-agent',
    project_id: 'test-project',
    workflow_id: 'feature',
    model: 'sonnet',
    timeout: 20,
    prompt: 'Test prompt',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('AgentDefForm - max_fail_restarts', () => {
  describe('form field rendering', () => {
    it('renders max_fail_restarts input with placeholder 0', () => {
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText('0')
      expect(input).toBeInTheDocument()
      expect(input).toHaveAttribute('type', 'number')
    })

    it('has min=0 and max=10 constraints', () => {
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText('0')
      expect(input).toHaveAttribute('min', '0')
      expect(input).toHaveAttribute('max', '10')
    })

    it('pre-fills value in edit mode', () => {
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ max_fail_restarts: 3 })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.getByPlaceholderText('0')).toHaveValue(3)
    })

    it('renders empty when undefined in initial data', () => {
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ max_fail_restarts: undefined })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.getByPlaceholderText('0')).toHaveValue(null)
    })
  })

  describe('create mode submission', () => {
    it('includes max_fail_restarts when set', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true}
          initial={{ id: 'new-agent', model: 'sonnet', timeout: 20, prompt: 'Test prompt' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText('0'), '3')
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ max_fail_restarts: 3 } as AgentDefCreateRequest)
      )
    })

    it('omits max_fail_restarts when empty (sends undefined)', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText('e.g., setup-analyzer'), 'new-agent')
      // leave max_fail_restarts empty
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ max_fail_restarts: undefined } as AgentDefCreateRequest)
      )
    })

    it('sends 0 when user explicitly types 0', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText('e.g., setup-analyzer'), 'new-agent')
      await user.type(screen.getByPlaceholderText('0'), '0')
      await user.click(screen.getByText('Create'))

      // 0 is a valid explicit value (means disabled), not empty string
      const submittedData = onSubmit.mock.calls[0][0]
      expect(submittedData.max_fail_restarts).toBe(0)
    })

    it('handles max value of 10', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText('e.g., setup-analyzer'), 'new-agent')
      await user.type(screen.getByPlaceholderText('0'), '10')
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ max_fail_restarts: 10 })
      )
    })
  })

  describe('edit mode submission', () => {
    it('includes max_fail_restarts in update request', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ max_fail_restarts: 2 })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText('0')
      await user.clear(input)
      await user.type(input, '5')
      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ max_fail_restarts: 5 } as AgentDefUpdateRequest)
      )
    })

    it('sends undefined when cleared', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ max_fail_restarts: 2 })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText('0')
      await user.clear(input)
      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ max_fail_restarts: undefined } as AgentDefUpdateRequest)
      )
    })

    it('preserves undefined when not modified', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ max_fail_restarts: undefined })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ max_fail_restarts: undefined })
      )
    })
  })
})
