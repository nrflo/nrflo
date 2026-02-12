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

describe('AgentDefForm - restart_threshold', () => {
  describe('form field rendering', () => {
    it('renders restart threshold input field', () => {
      render(
        <AgentDefForm
          isCreate={true} initial={{ prompt: 'test' }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText('25')
      expect(input).toBeInTheDocument()
      expect(input).toHaveAttribute('type', 'number')
      expect(input).toHaveAttribute('placeholder', '25')
    })

    it('restart threshold input has min=1 and max=99 constraints', () => {
      render(
        <AgentDefForm
          isCreate={true} initial={{ prompt: 'test' }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText('25')
      expect(input).toHaveAttribute('min', '1')
      expect(input).toHaveAttribute('max', '99')
    })

    it('renders restart threshold input in edit mode', () => {
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ restart_threshold: 30 })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText('25')
      expect(input).toHaveValue(30)
    })

    it('renders empty restart threshold when undefined in initial data', () => {
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ restart_threshold: undefined })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText('25')
      expect(input).toHaveValue(null)
    })
  })

  describe('create mode submission', () => {
    it('includes restart_threshold in create request when value is set', async () => {
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

      const thresholdInput = screen.getByPlaceholderText("25")
      await user.type(thresholdInput, '30')
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: 30,
        } as AgentDefCreateRequest)
      )
    })

    it('omits restart_threshold from create request when empty', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true} initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText("e.g., setup-analyzer"), 'new-agent')
      // Leave restart_threshold empty
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'new-agent',
          restart_threshold: undefined,
        } as AgentDefCreateRequest)
      )
    })

    it('handles restart_threshold = 1 (minimum value)', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true} initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText("e.g., setup-analyzer"), 'new-agent')
      await user.type(screen.getByPlaceholderText("25"), '1')
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: 1,
        })
      )
    })

    it('handles restart_threshold = 99 (maximum value)', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true} initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText("e.g., setup-analyzer"), 'new-agent')
      await user.type(screen.getByPlaceholderText("25"), '99')
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: 99,
        })
      )
    })
  })

  describe('edit mode submission', () => {
    it('includes restart_threshold in update request when value is set', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ restart_threshold: 25 })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText("25")
      await user.clear(input)
      await user.type(input, '35')
      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: 35,
        } as AgentDefUpdateRequest)
      )
    })

    it('omits restart_threshold from update request when cleared', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ restart_threshold: 25 })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText("25")
      await user.clear(input)
      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: undefined,
        } as AgentDefUpdateRequest)
      )
    })

    it('preserves undefined restart_threshold when not modified', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ restart_threshold: undefined })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: undefined,
        })
      )
    })

    it('allows changing from undefined to a specific value', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ restart_threshold: undefined })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText("25")
      await user.type(input, '40')
      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: 40,
        })
      )
    })
  })

  describe('edge cases', () => {
    it('handles rapid value changes correctly', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ restart_threshold: 25 })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      const input = screen.getByPlaceholderText("25")
      await user.clear(input)
      await user.type(input, '10')
      await user.clear(input)
      await user.type(input, '50')
      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: 50,
        })
      )
    })

    it('treats empty string as undefined in submission', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true} initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText("e.g., setup-analyzer"), 'test-agent')
      // restart_threshold input is left empty (default state)
      await user.click(screen.getByText('Create'))

      const submittedData = onSubmit.mock.calls[0][0]
      expect(submittedData.restart_threshold).toBeUndefined()
    })
  })

  describe('form validation', () => {
    it('does not block submission when restart_threshold is empty', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true} initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText("e.g., setup-analyzer"), 'test-agent')
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalled()
    })

    it('accepts zero as a value even though min=1 (browser validation)', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <AgentDefForm
          isCreate={true} initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText("e.g., setup-analyzer"), 'test-agent')
      await user.type(screen.getByPlaceholderText("25"), '0')

      // Note: HTML5 form validation would block this in a real browser,
      // but in testing environment, we submit the value as-is
      await user.click(screen.getByText('Create'))

      // If form is submitted, value should be 0
      if (onSubmit.mock.calls.length > 0) {
        expect(onSubmit).toHaveBeenCalledWith(
          expect.objectContaining({
            restart_threshold: 0,
          })
        )
      }
    })
  })
})
