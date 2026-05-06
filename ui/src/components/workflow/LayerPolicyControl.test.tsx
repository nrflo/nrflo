import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { LayerPolicyControl } from './LayerPolicyControl'
import type React from 'react'

const mockSetLayerPolicy = vi.fn()
const mockDeleteLayerPolicy = vi.fn()

vi.mock('@/api/workflowLayerPolicies', () => ({
  setLayerPolicy: (...args: unknown[]) => mockSetLayerPolicy(...args),
  deleteLayerPolicy: (...args: unknown[]) => mockDeleteLayerPolicy(...args),
  listLayerPolicies: vi.fn().mockResolvedValue({}),
}))

function getPolicyDropdownButton() {
  const label = screen.getByText(/Layer \d+ policy:/i)
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

async function selectDropdownOption(
  user: ReturnType<typeof userEvent.setup>,
  triggerButton: HTMLButtonElement,
  optionLabel: string
) {
  await user.click(triggerButton)
  const container = triggerButton.closest('.relative')!
  const option = Array.from(container.querySelectorAll('.cursor-pointer span')).find(
    (el) => el.textContent === optionLabel
  ) as HTMLElement
  await user.click(option)
}

function renderControl(props: Partial<React.ComponentProps<typeof LayerPolicyControl>> = {}) {
  return renderWithQuery(
    <LayerPolicyControl
      workflowId="wf-1"
      layer={0}
      agentCount={2}
      layerPoliciesQueryKey={['workflow-layer-policies', 'proj', 'wf-1']}
      {...props}
    />
  )
}

describe('LayerPolicyControl', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSetLayerPolicy.mockResolvedValue({ status: 'ok' })
    mockDeleteLayerPolicy.mockResolvedValue({ status: 'ok' })
  })

  it('defaults to "any" with no numeric input or Save button', () => {
    renderControl()
    expect(getPolicyDropdownButton().textContent).toContain('any (1 agent must pass)')
    expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /save/i })).not.toBeInTheDocument()
  })

  it('pre-selects kind and value when current policy is quorum:2', () => {
    renderControl({ current: 'quorum:2' })
    expect(getPolicyDropdownButton().textContent).toContain('quorum (N agents must pass)')
    expect(screen.getByRole('spinbutton')).toHaveValue(2)
    expect(screen.queryByRole('button', { name: /save/i })).not.toBeInTheDocument()
  })

  it('pre-selects percent kind when current policy is percent:75', () => {
    renderControl({ current: 'percent:75' })
    expect(getPolicyDropdownButton().textContent).toContain('percent (P% must pass)')
    expect(screen.getByRole('spinbutton')).toHaveValue(75)
  })

  describe('quorum policy', () => {
    it('shows numeric input after selecting quorum', async () => {
      const user = userEvent.setup()
      renderControl()
      await selectDropdownOption(user, getPolicyDropdownButton(), 'quorum (N agents must pass)')
      expect(screen.getByRole('spinbutton')).toBeInTheDocument()
    })

    it('calls setLayerPolicy(wf-1, 0, quorum:2) for valid N', async () => {
      const user = userEvent.setup()
      renderControl()
      await selectDropdownOption(user, getPolicyDropdownButton(), 'quorum (N agents must pass)')
      await user.type(screen.getByRole('spinbutton'), '2')
      await user.click(screen.getByRole('button', { name: /save/i }))
      expect(mockSetLayerPolicy).toHaveBeenCalledWith('wf-1', 0, 'quorum:2')
      expect(mockDeleteLayerPolicy).not.toHaveBeenCalled()
    })

    it('shows validation error and skips API when N exceeds agentCount', async () => {
      const user = userEvent.setup()
      renderControl({ agentCount: 2 })
      await selectDropdownOption(user, getPolicyDropdownButton(), 'quorum (N agents must pass)')
      await user.type(screen.getByRole('spinbutton'), '3')
      await user.click(screen.getByRole('button', { name: /save/i }))
      expect(screen.getByText(/N must be between 1 and 2/i)).toBeInTheDocument()
      expect(mockSetLayerPolicy).not.toHaveBeenCalled()
    })

    it('shows validation error when N is 0', async () => {
      const user = userEvent.setup()
      renderControl()
      await selectDropdownOption(user, getPolicyDropdownButton(), 'quorum (N agents must pass)')
      await user.type(screen.getByRole('spinbutton'), '0')
      await user.click(screen.getByRole('button', { name: /save/i }))
      expect(screen.getByText(/N must be between 1 and/i)).toBeInTheDocument()
      expect(mockSetLayerPolicy).not.toHaveBeenCalled()
    })
  })

  describe('percent policy', () => {
    it('shows numeric input after selecting percent', async () => {
      const user = userEvent.setup()
      renderControl()
      await selectDropdownOption(user, getPolicyDropdownButton(), 'percent (P% must pass)')
      expect(screen.getByRole('spinbutton')).toBeInTheDocument()
    })

    it('calls setLayerPolicy with percent:80 for valid P', async () => {
      const user = userEvent.setup()
      renderControl()
      await selectDropdownOption(user, getPolicyDropdownButton(), 'percent (P% must pass)')
      await user.type(screen.getByRole('spinbutton'), '80')
      await user.click(screen.getByRole('button', { name: /save/i }))
      expect(mockSetLayerPolicy).toHaveBeenCalledWith('wf-1', 0, 'percent:80')
    })

    it('shows validation error and skips API when P exceeds 100', async () => {
      const user = userEvent.setup()
      renderControl()
      await selectDropdownOption(user, getPolicyDropdownButton(), 'percent (P% must pass)')
      await user.type(screen.getByRole('spinbutton'), '101')
      await user.click(screen.getByRole('button', { name: /save/i }))
      expect(screen.getByText(/P must be between 1 and 100/i)).toBeInTheDocument()
      expect(mockSetLayerPolicy).not.toHaveBeenCalled()
    })
  })

  describe('all policy', () => {
    it('calls setLayerPolicy with all when all is selected', async () => {
      const user = userEvent.setup()
      renderControl()
      await selectDropdownOption(user, getPolicyDropdownButton(), 'all (every agent must pass)')
      await user.click(screen.getByRole('button', { name: /save/i }))
      expect(mockSetLayerPolicy).toHaveBeenCalledWith('wf-1', 0, 'all')
      expect(mockDeleteLayerPolicy).not.toHaveBeenCalled()
    })
  })

  it('calls deleteLayerPolicy when reverting to any from an existing policy', async () => {
    const user = userEvent.setup()
    renderControl({ current: 'all' })
    await selectDropdownOption(user, getPolicyDropdownButton(), 'any (1 agent must pass)')
    await user.click(screen.getByRole('button', { name: /save/i }))
    expect(mockDeleteLayerPolicy).toHaveBeenCalledWith('wf-1', 0)
    expect(mockSetLayerPolicy).not.toHaveBeenCalled()
  })
})
