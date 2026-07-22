import { describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import * as api from '@/api/customProviders'
import { createTestQueryClient, createWrapper } from '@/test/utils'
import {
  customProviderKeys,
  useCustomProviders,
  useCreateCustomProvider,
  useUpdateCustomProvider,
  useDeleteCustomProvider,
} from './useCustomProviders'
import type { CustomProvider } from '@/api/customProviders'

vi.mock('@/api/customProviders')

const provider: CustomProvider = {
  name: 'acme', base_url: 'https://acme.test', api_key: 'k', api_wire: 'responses', enabled: true, created_at: '', updated_at: '',
}

describe('customProviderKeys', () => {
  it('nests list() under all', () => {
    expect(customProviderKeys.list()).toEqual([...customProviderKeys.all, 'list'])
  })
})

describe('useCustomProviders', () => {
  it('fetches and returns the provider list', async () => {
    vi.mocked(api.listCustomProviders).mockResolvedValue([provider])
    const { result } = renderHook(() => useCustomProviders(), { wrapper: createWrapper(createTestQueryClient()) })
    await waitFor(() => expect(result.current.data).toEqual([provider]))
  })
})

describe('useCreateCustomProvider', () => {
  it('invalidates the provider list on success', async () => {
    vi.mocked(api.createCustomProvider).mockResolvedValue(provider)
    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useCreateCustomProvider(), { wrapper: createWrapper(queryClient) })
    result.current.mutate({ name: 'acme', base_url: 'https://acme.test', api_key: 'k', api_wire: 'responses' })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: customProviderKeys.list() })
  })
})

describe('useUpdateCustomProvider', () => {
  it('calls updateCustomProvider with name and data, invalidates on success', async () => {
    vi.mocked(api.updateCustomProvider).mockResolvedValue(provider)
    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useUpdateCustomProvider(), { wrapper: createWrapper(queryClient) })
    result.current.mutate({ name: 'acme', data: { enabled: false } })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(api.updateCustomProvider).toHaveBeenCalledWith('acme', { enabled: false })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: customProviderKeys.list() })
  })
})

describe('useDeleteCustomProvider', () => {
  it('invalidates the provider list on success', async () => {
    vi.mocked(api.deleteCustomProvider).mockResolvedValue({ status: 'ok' })
    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useDeleteCustomProvider(), { wrapper: createWrapper(queryClient) })
    result.current.mutate('acme')
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(api.deleteCustomProvider).toHaveBeenCalledWith('acme', expect.anything())
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: customProviderKeys.list() })
  })

  it('surfaces the BE 409 in-use error message on failure', async () => {
    vi.mocked(api.deleteCustomProvider).mockRejectedValue(new Error('custom provider is in use by: model-a'))
    const { result } = renderHook(() => useDeleteCustomProvider(), { wrapper: createWrapper(createTestQueryClient()) })
    result.current.mutate('acme')
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error?.message).toBe('custom provider is in use by: model-a')
  })
})
