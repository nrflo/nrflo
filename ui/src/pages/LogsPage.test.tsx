import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LogsPage } from './LogsPage'

vi.mock('./LogsFinishedTab', () => ({
  LogsFinishedTab: () => <div>finished-tab-content</div>,
}))
vi.mock('./LogsLiveTab', () => ({
  LogsLiveTab: () => <div>live-tab-content</div>,
}))

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <LogsPage />
    </QueryClientProvider>
  )
}

describe('LogsPage (tab shell)', () => {
  it('renders page heading', () => {
    renderPage()
    expect(screen.getByRole('heading', { name: 'Agent sessions' })).toBeInTheDocument()
  })

  it('renders both tab labels', () => {
    renderPage()
    expect(screen.getByRole('button', { name: 'Finished sessions' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Live processes' })).toBeInTheDocument()
  })

  it('shows Finished sessions tab content by default', () => {
    renderPage()
    expect(screen.getByText('finished-tab-content')).toBeInTheDocument()
    expect(screen.queryByText('live-tab-content')).not.toBeInTheDocument()
  })

  it('switches to Live processes tab on click', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Live processes' }))

    expect(screen.getByText('live-tab-content')).toBeInTheDocument()
    expect(screen.queryByText('finished-tab-content')).not.toBeInTheDocument()
  })

  it('switches back to Finished sessions tab', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Live processes' }))
    await user.click(screen.getByRole('button', { name: 'Finished sessions' }))

    expect(screen.getByText('finished-tab-content')).toBeInTheDocument()
    expect(screen.queryByText('live-tab-content')).not.toBeInTheDocument()
  })
})
