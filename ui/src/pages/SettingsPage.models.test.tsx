import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SettingsPage } from './SettingsPage'

vi.mock('@/stores/projectStore', () => ({ useProjectStore: (selector: (state: object) => unknown) => selector({ projects: [{ id: 'p1' }] }) }))
vi.mock('@/components/settings/ModelsList', () => ({ ModelsList: ({ provider }: { provider: string }) => <div data-testid="models-list" data-provider={provider} /> }))
vi.mock('@/components/settings/GlobalSettingsSection', () => ({ GlobalSettingsSection: () => null }))

function renderPage(search = '') {
  return render(<MemoryRouter initialEntries={[`/settings${search}`]}><SettingsPage /></MemoryRouter>)
}

describe('SettingsPage models tab', () => {
  it('uses one Models tab with Anthropic and OpenAI provider subtabs', async () => {
    const user = userEvent.setup()
    renderPage('?tab=models')
    expect(screen.getByTestId('models-list')).toHaveAttribute('data-provider', 'anthropic')
    expect(screen.queryByRole('button', { name: 'CLI Models' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'API Models' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'OpenAI' }))
    expect(screen.getByTestId('models-list')).toHaveAttribute('data-provider', 'openai')
  })
})
