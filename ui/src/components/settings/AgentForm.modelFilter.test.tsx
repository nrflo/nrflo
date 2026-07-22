import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SystemAgentsSection } from './SystemAgentsSection'
import * as systemAgentDefsApi from '@/api/systemAgentDefs'
import * as modelsApi from '@/api/models'
import { renderWithQuery } from '@/test/utils'
import type { Model } from '@/api/models'

vi.mock('@/api/systemAgentDefs')
vi.mock('@/api/models')

function makeModel(overrides: Partial<Model> = {}): Model {
  return {
    id: 'claude-both',
    provider: 'anthropic',
    display_name: 'Claude Both',
    cli_model: 'claude-cli',
    api_model: 'claude-api',
    cli_efforts: [],
    api_efforts: [],
    cli_context: 200000,
    api_context: 200000,
    fallback_models: '',
    default_effort: '',
    read_only: false,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

// anthropic row: valid in both cli and api modes
const anthropicBoth = makeModel()
// openrouter row: api-only (no cli_model), per model.go:96-101
const openrouterApiOnly = makeModel({
  id: 'or-api',
  provider: 'openrouter',
  display_name: 'OR Model',
  cli_model: '',
  api_model: 'or-api-1',
})
// openai row: cli-only (no api_model)
const openaiCliOnly = makeModel({
  id: 'gpt-cli',
  provider: 'openai',
  display_name: 'GPT CLI',
  cli_model: 'gpt-cli-x',
  api_model: '',
})

async function openCreateForm() {
  vi.mocked(systemAgentDefsApi.listSystemAgentDefs).mockResolvedValue([])
  vi.mocked(modelsApi.listModels).mockResolvedValue([anthropicBoth, openrouterApiOnly, openaiCliOnly])
  renderWithQuery(<SystemAgentsSection />)
  await screen.findByText('No system agents defined. Create one to get started.')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /New System Agent/ }))
  // Model dropdown only renders once the override toggle is switched on —
  // by default a new agent stays on its tier's fallback chain (model=='').
  await user.click(screen.getByRole('switch', { name: /Override model/ }))
  return user
}

function modeDropdownButton() {
  return screen.getByText('Mode').closest('div')!.querySelector('button') as HTMLButtonElement
}

function modelDropdownButton() {
  return screen.getByText('Model').closest('div')!.querySelector('button') as HTMLButtonElement
}

/** Returns the open options panel for the Model dropdown (distinct from the trigger button's own label). */
function modelOptionsPanel() {
  const panel = modelDropdownButton().parentElement!.querySelector('.absolute') as HTMLElement
  return within(panel)
}

describe('AgentForm — model dropdown reactivity to execution_mode', () => {
  beforeEach(() => vi.clearAllMocks())

  it('filters out api-only models in CLI mode and shows them after switching to API', async () => {
    const user = await openCreateForm()

    // CLI Interactive is the default mode — openrouter (api-only) must not be selectable
    await user.click(modelDropdownButton())
    expect(modelOptionsPanel().getByText('Anthropic: Claude Both')).toBeInTheDocument()
    expect(modelOptionsPanel().getByText('OpenAI: GPT CLI')).toBeInTheDocument()
    expect(modelOptionsPanel().queryByText('OpenRouter: OR Model')).not.toBeInTheDocument()
    await user.keyboard('{Escape}')

    // Switch mode to API
    await user.click(modeDropdownButton())
    await user.click(screen.getByText('API'))

    // Now the api-only openrouter model is selectable, and the cli-only openai model is gone
    await user.click(modelDropdownButton())
    expect(modelOptionsPanel().getByText('OpenRouter: OR Model')).toBeInTheDocument()
    expect(modelOptionsPanel().getByText('Anthropic: Claude Both')).toBeInTheDocument()
    expect(modelOptionsPanel().queryByText('OpenAI: GPT CLI')).not.toBeInTheDocument()
    await user.keyboard('{Escape}')

    // Switch back to CLI Interactive — openrouter disappears again
    await user.click(modeDropdownButton())
    await user.click(screen.getByText('CLI Interactive'))
    await user.click(modelDropdownButton())
    expect(modelOptionsPanel().queryByText('OpenRouter: OR Model')).not.toBeInTheDocument()
  })

  it('resets the selected model when it becomes invalid for the newly selected mode', async () => {
    const user = await openCreateForm()

    // Pick the cli-only model while in CLI mode
    await user.click(modelDropdownButton())
    await user.click(modelOptionsPanel().getByText('OpenAI: GPT CLI'))
    expect(within(modelDropdownButton()).getByText('OpenAI: GPT CLI')).toBeInTheDocument()

    // Switching to API mode invalidates 'gpt-cli' (no api_model) — the form must reset the selection
    await user.click(modeDropdownButton())
    await user.click(screen.getByText('API'))

    expect(within(modelDropdownButton()).queryByText('OpenAI: GPT CLI')).not.toBeInTheDocument()
  })
})
