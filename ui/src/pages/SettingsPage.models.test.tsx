import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SettingsPage } from './SettingsPage'

vi.mock('@/stores/projectStore', () => ({ useProjectStore: (selector: (state: object) => unknown) => selector({ projects: [{ id: 'p1' }] }) }))
vi.mock('@/components/settings/ModelsList', () => ({ ModelsList: ({ provider }: { provider: string }) => <div data-testid="models-list" data-provider={provider} /> }))
vi.mock('@/components/settings/GlobalSettingsSection', () => ({ GlobalSettingsSection: () => null }))
let customProviders: { name: string }[] = []
vi.mock('@/hooks/useCustomProviders', () => ({
  useCustomProviders: () => ({ data: customProviders, isLoading: false, error: null }),
  useCreateCustomProvider: () => ({ mutate: vi.fn(), isPending: false, isError: false, error: null }),
}))

function renderPage(search = '') {
  return render(<MemoryRouter initialEntries={[`/settings${search}`]}><SettingsPage /></MemoryRouter>)
}

describe('SettingsPage models tab', () => {
  beforeEach(() => { customProviders = [] })

  it('uses one Models tab with Anthropic and OpenAI provider subtabs', async () => {
    const user = userEvent.setup()
    renderPage('?tab=models')
    expect(screen.getByTestId('models-list')).toHaveAttribute('data-provider', 'anthropic')
    expect(screen.queryByRole('button', { name: 'CLI Models' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'API Models' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'OpenAI' }))
    expect(screen.getByTestId('models-list')).toHaveAttribute('data-provider', 'openai')
  })

  it('routes the OpenRouter subtab to ModelsList', async () => {
    const user = userEvent.setup()
    renderPage('?tab=models')
    await user.click(screen.getByRole('button', { name: 'OpenRouter' }))
    expect(screen.getByTestId('models-list')).toHaveAttribute('data-provider', 'openrouter')
  })

  it('renders ModelsList directly from ?sub=openrouter', () => {
    renderPage('?tab=models&sub=openrouter')
    expect(screen.getByTestId('models-list')).toHaveAttribute('data-provider', 'openrouter')
  })

  it('renders a registered custom provider as a dynamic subtab and routes ModelsList to it', async () => {
    customProviders = [{ name: 'acme' }]
    const user = userEvent.setup()
    renderPage('?tab=models')
    expect(screen.getByRole('button', { name: 'acme' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'acme' }))
    expect(screen.getByTestId('models-list')).toHaveAttribute('data-provider', 'acme')
  })

  it('falls back to anthropic when ?sub= names a provider that no longer exists', () => {
    renderPage('?tab=models&sub=stale-provider')
    expect(screen.getByTestId('models-list')).toHaveAttribute('data-provider', 'anthropic')
  })

  it('the Add-provider affordance opens CustomProviderForm', async () => {
    const user = userEvent.setup()
    renderPage('?tab=models')
    expect(screen.queryByPlaceholderText('my-provider')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /add provider/i }))
    expect(screen.getByPlaceholderText('my-provider')).toBeInTheDocument()
  })
})
