import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import { ProvidersSection } from './ProvidersSection'

vi.mock('./ProviderModelsList', () => ({
  ProviderModelsList: ({ provider }: { provider: string }) => (
    <div data-testid="provider-models-list" data-provider={provider} />
  ),
}))

describe('ProvidersSection', () => {
  it('passes activeProvider to ProviderModelsList', () => {
    renderWithQuery(<ProvidersSection activeProvider="gemini" />)
    const list = screen.getByTestId('provider-models-list')
    expect(list).toHaveAttribute('data-provider', 'gemini')
  })
})
