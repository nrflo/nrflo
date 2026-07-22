import { describe, expect, it, vi, afterEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProviderConnectionCheckButton } from './ProviderConnectionCheckButton'
import * as customProvidersApi from '@/api/customProviders'

vi.mock('@/api/customProviders')

afterEach(() => vi.useRealTimers())

describe('ProviderConnectionCheckButton', () => {
  it('is disabled when baseUrl is blank', () => {
    render(<ProviderConnectionCheckButton baseUrl="" apiKey="k" apiWire="responses" />)
    expect(screen.getByRole('button', { name: /check connection/i })).toBeDisabled()
  })

  it('calls checkCustomProviderConnection with the current fields', async () => {
    vi.mocked(customProvidersApi.checkCustomProviderConnection).mockResolvedValue({ ok: true, models: ['a'] })
    const user = userEvent.setup()
    render(<ProviderConnectionCheckButton baseUrl="https://api.test" apiKey="sk-1" apiWire="chat_completions" />)
    await user.click(screen.getByRole('button', { name: /check connection/i }))
    expect(customProvidersApi.checkCustomProviderConnection).toHaveBeenCalledWith(
      { base_url: 'https://api.test', api_key: 'sk-1', api_wire: 'chat_completions' },
      expect.any(AbortSignal),
    )
  })

  it('shows model count on success', async () => {
    vi.mocked(customProvidersApi.checkCustomProviderConnection).mockResolvedValue({ ok: true, models: ['a', 'b'] })
    const user = userEvent.setup()
    render(<ProviderConnectionCheckButton baseUrl="https://api.test" apiKey="sk-1" apiWire="responses" />)
    await user.click(screen.getByRole('button', { name: /check connection/i }))
    expect(await screen.findByText('2 models')).toBeInTheDocument()
  })

  it('opens an error dialog when the server reports ok: false', async () => {
    vi.mocked(customProvidersApi.checkCustomProviderConnection).mockResolvedValue({ ok: false, models: [], error: 'invalid api key' })
    const user = userEvent.setup()
    render(<ProviderConnectionCheckButton baseUrl="https://api.test" apiKey="bad" apiWire="responses" />)
    await user.click(screen.getByRole('button', { name: /check connection/i }))
    expect(await screen.findByText('invalid api key')).toBeInTheDocument()
  })

  it('opens an error dialog when the request rejects', async () => {
    vi.mocked(customProvidersApi.checkCustomProviderConnection).mockRejectedValue(new Error('network down'))
    const user = userEvent.setup()
    render(<ProviderConnectionCheckButton baseUrl="https://api.test" apiKey="k" apiWire="responses" />)
    await user.click(screen.getByRole('button', { name: /check connection/i }))
    expect(await screen.findByText('network down')).toBeInTheDocument()
  })

  it('shows a timeout message after 45s with no response', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    vi.mocked(customProvidersApi.checkCustomProviderConnection).mockImplementation((_req, signal) =>
      new Promise((_, reject) => {
        signal?.addEventListener('abort', () => reject(new DOMException('The operation was aborted.', 'AbortError')))
      }),
    )
    render(<ProviderConnectionCheckButton baseUrl="https://api.test" apiKey="k" apiWire="responses" />)
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /check connection/i }))
    })
    await act(async () => {
      vi.advanceTimersByTime(45_000)
    })
    expect(screen.getByText('Timeout — server did not respond')).toBeInTheDocument()
  })

  it('closes the error dialog when Close is clicked', async () => {
    vi.mocked(customProvidersApi.checkCustomProviderConnection).mockResolvedValue({ ok: false, models: [], error: 'boom' })
    const user = userEvent.setup()
    render(<ProviderConnectionCheckButton baseUrl="https://api.test" apiKey="k" apiWire="responses" />)
    await user.click(screen.getByRole('button', { name: /check connection/i }))
    expect(await screen.findByText('boom')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Close' }))
    expect(screen.queryByText('boom')).not.toBeInTheDocument()
  })
})
