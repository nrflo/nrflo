import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useSearchParams } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LogsPage } from './LogsPage'

vi.mock('./LogsFinishedTab', () => ({
  LogsFinishedTab: () => <div>finished-tab-content</div>,
}))
vi.mock('./LogsLiveTab', () => ({
  LogsLiveTab: () => <div>live-tab-content</div>,
}))
vi.mock('./LogsSessionsTab', () => ({
  LogsSessionsTab: () => <div>sessions-tab-content</div>,
}))

function SearchParamsProbe() {
  const [params] = useSearchParams()
  return <div data-testid="search-params">{params.toString()}</div>
}

function renderPage(initialEntry = '/logs') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <LogsPage />
        <SearchParamsProbe />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('LogsPage (tab shell)', () => {
  it('renders page heading', () => {
    renderPage()
    expect(screen.getByRole('heading', { name: 'Agent sessions' })).toBeInTheDocument()
  })

  it('renders all tab labels', () => {
    renderPage()
    expect(screen.getByRole('button', { name: 'Sessions' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Finished sessions' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Live processes' })).toBeInTheDocument()
  })

  it('shows Sessions tab content by default', () => {
    renderPage()
    expect(screen.getByText('sessions-tab-content')).toBeInTheDocument()
    expect(screen.queryByText('finished-tab-content')).not.toBeInTheDocument()
    expect(screen.queryByText('live-tab-content')).not.toBeInTheDocument()
  })

  it('switches to Live processes tab on click', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Live processes' }))

    expect(screen.getByText('live-tab-content')).toBeInTheDocument()
    expect(screen.queryByText('sessions-tab-content')).not.toBeInTheDocument()
  })

  it('switches to Finished sessions tab and back to Sessions', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Finished sessions' }))
    expect(screen.getByText('finished-tab-content')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Sessions' }))
    expect(screen.getByText('sessions-tab-content')).toBeInTheDocument()
    expect(screen.queryByText('finished-tab-content')).not.toBeInTheDocument()
  })

  it('deep links directly to the finished tab via ?tab=finished', () => {
    renderPage('/logs?tab=finished')
    expect(screen.getByText('finished-tab-content')).toBeInTheDocument()
    expect(screen.queryByText('sessions-tab-content')).not.toBeInTheDocument()
  })

  it('falls back to the Sessions tab for an unrecognized ?tab= value', () => {
    renderPage('/logs?tab=bogus')
    expect(screen.getByText('sessions-tab-content')).toBeInTheDocument()
  })

  it('sets ?tab= in the URL when switching tabs', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Live processes' }))
    expect(screen.getByTestId('search-params')).toHaveTextContent('tab=live')
  })

  it('preserves ?sid= while switching between Sessions views but clears it when leaving Sessions', async () => {
    const user = userEvent.setup()
    renderPage('/logs?tab=sessions&sid=session-1')
    expect(screen.getByTestId('search-params')).toHaveTextContent('sid=session-1')

    await user.click(screen.getByRole('button', { name: 'Finished sessions' }))
    expect(screen.getByTestId('search-params')).not.toHaveTextContent('sid=session-1')
  })
})
