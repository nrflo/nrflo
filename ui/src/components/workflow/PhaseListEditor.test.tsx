import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PhaseListEditor, type PhaseFormEntry } from './PhaseListEditor'

function renderEditor(
  props: Partial<React.ComponentProps<typeof PhaseListEditor>> = {}
) {
  const defaultProps = {
    value: [] as PhaseFormEntry[],
    onChange: vi.fn(),
    ...props,
  }
  return {
    ...render(<PhaseListEditor {...defaultProps} />),
    props: defaultProps,
  }
}

describe('PhaseListEditor', () => {
  describe('layer input rendering', () => {
    it('renders layer input field for each agent', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'implementor', layer: 1 },
      ]
      renderEditor({ value: entries })

      const layerInputs = screen.getAllByDisplayValue(/^[01]$/)
      expect(layerInputs).toHaveLength(2)
      expect(layerInputs[0]).toHaveValue(0)
      expect(layerInputs[1]).toHaveValue(1)
    })

    it('updates layer value when changed', async () => {
      const user = userEvent.setup()
      const entries: PhaseFormEntry[] = [
        { agent: 'setup-analyzer', layer: 0 },
      ]
      const onChange = vi.fn()
      renderEditor({ value: entries, onChange })

      const layerInput = screen.getByDisplayValue('0')
      await user.clear(layerInput)
      await user.type(layerInput, '2')

      expect(onChange).toHaveBeenCalledWith([
        { agent: 'setup-analyzer', layer: 2 },
      ])
    })

    it('adds new agent with max_layer+1 as default', async () => {
      const user = userEvent.setup()
      const entries: PhaseFormEntry[] = [
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'implementor', layer: 2 },
      ]
      const onChange = vi.fn()
      renderEditor({ value: entries, onChange })

      const addButton = screen.getByRole('button', { name: /add agent/i })
      await user.click(addButton)

      expect(onChange).toHaveBeenCalledWith([
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'implementor', layer: 2 },
        { agent: '', layer: 3 },
      ])
    })

    it('adds first agent with layer 0 when list is empty', async () => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      renderEditor({ value: [], onChange })

      const addButton = screen.getByRole('button', { name: /add agent/i })
      await user.click(addButton)

      expect(onChange).toHaveBeenCalledWith([{ agent: '', layer: 0 }])
    })
  })

  describe('layer grouping and sorting', () => {
    it('displays layer headers for each distinct layer', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'implementor', layer: 1 },
        { agent: 'qa-verifier', layer: 2 },
      ]
      renderEditor({ value: entries })

      expect(screen.getByText('Layer 0')).toBeInTheDocument()
      expect(screen.getByText('Layer 1')).toBeInTheDocument()
      expect(screen.getByText('Layer 2')).toBeInTheDocument()
    })

    it('sorts entries by layer visually (ascending order)', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'implementor', layer: 2 },
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'qa-verifier', layer: 1 },
      ]
      renderEditor({ value: entries })

      // Verify visual order by checking the order of agent input placeholders
      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      expect(agentInputs[0]).toHaveValue('setup-analyzer')
      expect(agentInputs[1]).toHaveValue('qa-verifier')
      expect(agentInputs[2]).toHaveValue('implementor')
    })

    it('groups multiple agents in the same layer under one header', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'analyzer-a', layer: 0 },
        { agent: 'analyzer-b', layer: 0 },
        { agent: 'implementor', layer: 1 },
      ]
      renderEditor({ value: entries })

      const layerHeaders = screen.getAllByText(/Layer \d/)
      expect(layerHeaders).toHaveLength(2) // Only 2 distinct layers (0 and 1)

      // Both agents in layer 0 should be present
      expect(screen.getByDisplayValue('analyzer-a')).toBeInTheDocument()
      expect(screen.getByDisplayValue('analyzer-b')).toBeInTheDocument()
    })
  })

  describe('fan-in validation', () => {
    it('shows no error when fan-in rule is satisfied', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'analyzer-a', layer: 0 },
        { agent: 'analyzer-b', layer: 0 },
        { agent: 'implementor', layer: 1 }, // Single agent after multi-agent layer
      ]
      renderEditor({ value: entries })

      expect(screen.queryByText(/fan-in violation/i)).not.toBeInTheDocument()
    })

    it('shows error when multi-agent layer is followed by multi-agent layer', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'analyzer-a', layer: 0 },
        { agent: 'analyzer-b', layer: 0 },
        { agent: 'implementor-a', layer: 1 },
        { agent: 'implementor-b', layer: 1 },
      ]
      renderEditor({ value: entries })

      expect(screen.getByText(/fan-in violation/i)).toBeInTheDocument()
      expect(screen.getByText(/layer 1 must have exactly 1 agent/i)).toBeInTheDocument()
    })

    it('shows error message inline near the offending layer', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'analyzer-a', layer: 0 },
        { agent: 'analyzer-b', layer: 0 },
        { agent: 'impl-a', layer: 1 },
        { agent: 'impl-b', layer: 1 },
      ]
      renderEditor({ value: entries })

      // Error should be shown in Layer 1 header area
      const errorMsg = screen.getByText(/fan-in violation.*layer 1/i)
      expect(errorMsg).toBeInTheDocument()

      // Error should have destructive styling
      expect(errorMsg.className).toContain('text-destructive')
    })

    it('no error when single agent layer follows single agent layer', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'implementor', layer: 1 },
        { agent: 'qa-verifier', layer: 2 },
      ]
      renderEditor({ value: entries })

      expect(screen.queryByText(/fan-in violation/i)).not.toBeInTheDocument()
    })

    it('no error when multi-agent layer is the last layer', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'impl-a', layer: 1 },
        { agent: 'impl-b', layer: 1 },
      ]
      renderEditor({ value: entries })

      expect(screen.queryByText(/fan-in violation/i)).not.toBeInTheDocument()
    })

    it('shows error with specific layer numbers in message', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'a1', layer: 3 },
        { agent: 'a2', layer: 3 },
        { agent: 'b1', layer: 5 },
        { agent: 'b2', layer: 5 },
      ]
      renderEditor({ value: entries })

      const errorMsg = screen.getByText(/fan-in violation/i)
      expect(errorMsg.textContent).toContain('layer 5')
      expect(errorMsg.textContent).toContain('layer 3')
      expect(errorMsg.textContent).toContain('2 agents')
    })
  })

  describe('agent input and removal', () => {
    it('updates agent name when typed', async () => {
      const user = userEvent.setup()
      const entries: PhaseFormEntry[] = [{ agent: '', layer: 0 }]
      const onChange = vi.fn()
      renderEditor({ value: entries, onChange })

      const agentInput = screen.getByPlaceholderText(/agent type/i)
      await user.type(agentInput, 'test')

      // userEvent.type triggers onChange for each character
      // Verify onChange was called and last call contains the last typed character
      expect(onChange).toHaveBeenCalled()
      const lastCall = onChange.mock.calls[onChange.mock.calls.length - 1]
      expect(lastCall[0][0].agent).toContain('t')
    })

    it('removes agent when trash button clicked', async () => {
      const user = userEvent.setup()
      const entries: PhaseFormEntry[] = [
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'implementor', layer: 1 },
      ]
      const onChange = vi.fn()
      renderEditor({ value: entries, onChange })

      const removeButtons = screen.getAllByTitle(/remove agent/i)
      await user.click(removeButtons[0])

      expect(onChange).toHaveBeenCalledWith([
        { agent: 'implementor', layer: 1 },
      ])
    })
  })

  describe('edge cases', () => {
    it('handles empty phases array', () => {
      renderEditor({ value: [] })

      const addButton = screen.getByRole('button', { name: /add agent/i })
      expect(addButton).toBeInTheDocument()
    })

    it('handles single agent per layer', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'setup-analyzer', layer: 0 },
        { agent: 'implementor', layer: 1 },
        { agent: 'qa-verifier', layer: 2 },
      ]
      renderEditor({ value: entries })

      expect(screen.getByText('Layer 0')).toBeInTheDocument()
      expect(screen.getByText('Layer 1')).toBeInTheDocument()
      expect(screen.getByText('Layer 2')).toBeInTheDocument()
      expect(screen.queryByText(/fan-in violation/i)).not.toBeInTheDocument()
    })

    it('handles all agents in layer 0', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'analyzer-a', layer: 0 },
        { agent: 'analyzer-b', layer: 0 },
        { agent: 'analyzer-c', layer: 0 },
      ]
      renderEditor({ value: entries })

      // All three agents should be in Layer 0, but the header is only shown once
      expect(screen.getByText('Layer 0')).toBeInTheDocument()
      expect(screen.queryByText('Layer 1')).not.toBeInTheDocument()
      expect(screen.queryByText(/fan-in violation/i)).not.toBeInTheDocument()
    })

    it('displays AlertTriangle icon with fan-in error', () => {
      const entries: PhaseFormEntry[] = [
        { agent: 'a1', layer: 0 },
        { agent: 'a2', layer: 0 },
        { agent: 'b1', layer: 1 },
        { agent: 'b2', layer: 1 },
      ]
      const { container } = renderEditor({ value: entries })

      // AlertTriangle icon should be rendered
      const iconSvg = container.querySelector('svg')
      expect(iconSvg).toBeInTheDocument()

      // Error message should be present
      expect(screen.getByText(/fan-in violation/i)).toBeInTheDocument()
    })
  })
})
