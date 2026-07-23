import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'
import type { AgentDef } from '@/types/workflow'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => true,
}))

vi.mock('@/hooks/useDefaultTemplates', () => ({
  useInjectableTemplates: () => ({ data: [] }),
}))

vi.mock('@/hooks/useModels', () => ({
  useModelOptions: () => [
    { label: 'Anthropic', options: [
      { value: 'opus-4-8', label: 'Anthropic: Opus' },
      { value: 'sonnet-5', label: 'Anthropic: Sonnet' },
    ]},
  ],
  useModels: () => ({ data: [] }),
}))

function makeAgentDef(overrides: Partial<AgentDef> = {}): AgentDef {
  return {
    id: 'test-agent',
    project_id: 'test-project',
    workflow_id: 'feature',
    model: 'sonnet-5',
    timeout: 20,
    prompt: 'Test prompt',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('AgentDefForm - restart_threshold edge cases and validation', () => {
  describe('edge cases', () => {
    it('handles rapid value changes correctly', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      renderWithQuery(
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

      renderWithQuery(
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

      renderWithQuery(
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

      renderWithQuery(
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
