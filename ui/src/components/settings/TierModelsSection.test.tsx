import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TierModelsSection } from './TierModelsSection'
import * as tierModelsApi from '@/api/tierModels'
import * as modelsApi from '@/api/models'
import { renderWithQuery } from '@/test/utils'
import type { TierModel } from '@/api/tierModels'
import type { Model } from '@/api/models'

vi.mock('@/api/tierModels')
vi.mock('@/api/models')

function makeModel(overrides: Partial<Model> = {}): Model {
  return {
    id: 'm-alpha',
    provider: 'anthropic',
    display_name: 'Model Alpha',
    cli_model: 'alpha-cli',
    api_model: 'alpha-api',
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

function makeRow(overrides: Partial<TierModel> = {}): TierModel {
  return {
    tier: 2,
    position: 0,
    provider: 'anthropic',
    execution_mode: 'cli_interactive',
    model_id: 'm-alpha',
    reasoning_effort: '',
    ...overrides,
  }
}

const modelAlpha = makeModel()
const modelBeta = makeModel({ id: 'm-beta', display_name: 'Model Beta' })

function tierSection(tier: number) {
  return screen.getByText(`Tier ${tier}`).closest('div.border')!
}

describe('TierModelsSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(modelsApi.listModels).mockResolvedValue([modelAlpha, modelBeta])
  })

  it('empty tier with nothing to inherit shows the no-chain note; a populated tier renders its own entries; a tier above it shows the inherited chain greyed', async () => {
    vi.mocked(tierModelsApi.listTierModels).mockResolvedValue([
      makeRow({ tier: 2, position: 0, model_id: 'm-alpha' }),
    ])
    renderWithQuery(<TierModelsSection />)
    await screen.findByText('Tier 1')

    // Tier 1: nothing saved anywhere below/at it — no chain to inherit
    expect(within(tierSection(1)).getByText(/No chain available to inherit/)).toBeInTheDocument()

    // Tier 2: has its own saved row — editable view, not the inherited note
    expect(within(tierSection(2)).getByText(/Anthropic: Model Alpha/)).toBeInTheDocument()
    expect(within(tierSection(2)).queryByText(/inherits from the nearest/)).not.toBeInTheDocument()

    // Tier 3: nothing saved — inherits tier 2's chain, greyed, with a provenance note
    expect(within(tierSection(3)).getByText(/inherits from the nearest lower populated tier/)).toBeInTheDocument()
    expect(within(tierSection(3)).getByText(/from tier 2/)).toBeInTheDocument()
  })

  it('add entry appends an editable row; remove drops it', async () => {
    vi.mocked(tierModelsApi.listTierModels).mockResolvedValue([])
    renderWithQuery(<TierModelsSection />)
    await screen.findByText('Tier 1')
    const user = userEvent.setup()

    const tier1 = within(tierSection(1))
    expect(tier1.getByText(/No chain available to inherit/)).toBeInTheDocument()

    await user.click(tier1.getByRole('button', { name: /Add entry/ }))
    expect(tier1.getByText('Primary')).toBeInTheDocument()
    expect(tier1.getByText('Select a model')).toBeInTheDocument()

    await user.click(tier1.getByRole('button', { name: /Add entry/ }))
    expect(tier1.getByText('Fallback 1')).toBeInTheDocument()

    await user.click(tier1.getByRole('button', { name: /Remove entry 1/ }))
    // The removed row was position 0 — the remaining row shifts up to "Primary"
    expect(tier1.queryByText('Fallback 1')).not.toBeInTheDocument()
    expect(tier1.getByText('Primary')).toBeInTheDocument()
  })

  it('reorders entries with the up/down chevrons and saves the new order', async () => {
    vi.mocked(tierModelsApi.listTierModels).mockResolvedValue([
      makeRow({ position: 0, model_id: 'm-alpha' }),
      makeRow({ position: 1, model_id: 'm-beta' }),
    ])
    const mutate = vi.fn()
    vi.mocked(tierModelsApi.setTierChain).mockImplementation(async () => {
      mutate()
      return { status: 'ok' }
    })
    renderWithQuery(<TierModelsSection />)
    await screen.findByText('Tier 1')
    const user = userEvent.setup()

    const tier2 = within(tierSection(2))
    // Primary is Alpha, Fallback 1 is Beta before reordering
    const rows = tier2.getAllByText(/^Anthropic: Model (Alpha|Beta)$/)
    expect(rows[0]).toHaveTextContent('Model Alpha')
    expect(rows[1]).toHaveTextContent('Model Beta')

    await user.click(tier2.getByRole('button', { name: 'Move entry 2 up' }))

    const reordered = tier2.getAllByText(/^Anthropic: Model (Alpha|Beta)$/)
    expect(reordered[0]).toHaveTextContent('Model Beta')
    expect(reordered[1]).toHaveTextContent('Model Alpha')

    await user.click(tier2.getByRole('button', { name: 'Save' }))
    expect(tierModelsApi.setTierChain).toHaveBeenCalledWith(2, [
      { execution_mode: 'cli_interactive', model_id: 'm-beta', reasoning_effort: '' },
      { execution_mode: 'cli_interactive', model_id: 'm-alpha', reasoning_effort: '' },
    ])
  })
})
