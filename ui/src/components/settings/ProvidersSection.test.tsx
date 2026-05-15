import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { ProvidersSection } from './ProvidersSection'

vi.mock('./ProviderModelsList', () => ({
  ProviderModelsList: ({ provider }: { provider: string }) => (
    <div data-testid="provider-models-list" data-provider={provider} />
  ),
}))

vi.mock('@/api/settings', () => ({
  settingsKeys: { all: ['settings'], global: () => ['settings', 'global'] },
  getGlobalSettings: vi.fn().mockResolvedValue({
    sync_claude_limits: false,
    low_consumption_mode: false,
    api_mode_enabled: false,
  }),
  updateGlobalSettings: vi.fn().mockResolvedValue(undefined),
}))

import * as settingsApi from '@/api/settings'

describe('ProvidersSection', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders Sync Claude limits toggle when activeProvider is claude', async () => {
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    expect(await screen.findByText('Sync Claude limits')).toBeInTheDocument()
    expect(screen.getByRole('switch')).toBeInTheDocument()
  })

  it('does not render Sync Claude limits toggle for opencode', () => {
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.queryByText('Sync Claude limits')).not.toBeInTheDocument()
  })

  it('does not render Sync Claude limits toggle for codex', () => {
    renderWithQuery(<ProvidersSection activeProvider="codex" />)
    expect(screen.queryByText('Sync Claude limits')).not.toBeInTheDocument()
  })

  it('does not render Sync Claude limits toggle for gemini', () => {
    renderWithQuery(<ProvidersSection activeProvider="gemini" />)
    expect(screen.queryByText('Sync Claude limits')).not.toBeInTheDocument()
  })

  it('passes activeProvider to ProviderModelsList', () => {
    renderWithQuery(<ProvidersSection activeProvider="gemini" />)
    const list = screen.getByTestId('provider-models-list')
    expect(list).toHaveAttribute('data-provider', 'gemini')
  })

  it('sync limits toggle calls updateGlobalSettings with toggled value', async () => {
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    const toggle = await screen.findByRole('switch')
    await userEvent.click(toggle)

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ sync_claude_limits: true })
    })
  })
})
