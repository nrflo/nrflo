import { renderHook } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useGoBack } from '../useGoBack'
import type { NavigateFunction, Location } from 'react-router-dom'

// Mock react-router-dom
vi.mock('react-router-dom', () => ({
  useNavigate: vi.fn(),
  useLocation: vi.fn(),
}))

import { useNavigate, useLocation } from 'react-router-dom'

const mockedUseNavigate = vi.mocked(useNavigate)
const mockedUseLocation = vi.mocked(useLocation)

describe('useGoBack', () => {
  let mockNavigate: NavigateFunction

  beforeEach(() => {
    mockNavigate = vi.fn() as unknown as NavigateFunction
    mockedUseNavigate.mockReturnValue(mockNavigate)
  })

  it('should call navigate(-1) when location.key is not "default"', () => {
    // Simulate browser history exists (location.key is some unique value)
    mockedUseLocation.mockReturnValue({
      key: 'abc123',
      pathname: '/current',
      search: '',
      hash: '',
      state: null,
    } as Location)

    const { result } = renderHook(() => useGoBack('/fallback'))
    const goBack = result.current

    goBack()

    expect(mockNavigate).toHaveBeenCalledWith(-1)
    expect(mockNavigate).toHaveBeenCalledTimes(1)
  })

  it('should call navigate(fallbackPath, {replace: true}) when location.key is "default"', () => {
    // Simulate no browser history (direct URL access)
    mockedUseLocation.mockReturnValue({
      key: 'default',
      pathname: '/current',
      search: '',
      hash: '',
      state: null,
    } as Location)

    const { result } = renderHook(() => useGoBack('/tickets'))
    const goBack = result.current

    goBack()

    expect(mockNavigate).toHaveBeenCalledWith('/tickets', { replace: true })
    expect(mockNavigate).toHaveBeenCalledTimes(1)
  })

  it('should use the provided fallback path when no history exists', () => {
    mockedUseLocation.mockReturnValue({
      key: 'default',
      pathname: '/tickets/ABC-123',
      search: '',
      hash: '',
      state: null,
    } as Location)

    const { result } = renderHook(() => useGoBack('/chains'))
    const goBack = result.current

    goBack()

    expect(mockNavigate).toHaveBeenCalledWith('/chains', { replace: true })
  })

  it('should return a memoized function that updates when dependencies change', () => {
    mockedUseLocation.mockReturnValue({
      key: 'abc123',
      pathname: '/current',
      search: '',
      hash: '',
      state: null,
    } as Location)

    const { result, rerender } = renderHook(
      ({ fallback }) => useGoBack(fallback),
      { initialProps: { fallback: '/tickets' } }
    )

    const goBack1 = result.current

    // Same location.key and fallback should return same function reference
    rerender({ fallback: '/tickets' })
    const goBack2 = result.current

    expect(goBack1).toBe(goBack2)

    // Different fallback should return new function
    rerender({ fallback: '/chains' })
    const goBack3 = result.current

    expect(goBack1).not.toBe(goBack3)
  })

  it('should handle location key changes correctly', () => {
    // Start with history
    mockedUseLocation.mockReturnValue({
      key: 'abc123',
      pathname: '/current',
      search: '',
      hash: '',
      state: null,
    } as Location)

    const { result, rerender } = renderHook(() => useGoBack('/tickets'))

    result.current()
    expect(mockNavigate).toHaveBeenCalledWith(-1)

    vi.clearAllMocks()

    // Change to no history (default key)
    mockedUseLocation.mockReturnValue({
      key: 'default',
      pathname: '/current',
      search: '',
      hash: '',
      state: null,
    } as Location)

    rerender()

    result.current()
    expect(mockNavigate).toHaveBeenCalledWith('/tickets', { replace: true })
  })

  it('should handle encoded URLs in fallback path', () => {
    mockedUseLocation.mockReturnValue({
      key: 'default',
      pathname: '/current',
      search: '',
      hash: '',
      state: null,
    } as Location)

    const encodedTicketPath = `/tickets/${encodeURIComponent('TICKET-123')}`
    const { result } = renderHook(() => useGoBack(encodedTicketPath))

    result.current()

    expect(mockNavigate).toHaveBeenCalledWith(encodedTicketPath, { replace: true })
  })
})
