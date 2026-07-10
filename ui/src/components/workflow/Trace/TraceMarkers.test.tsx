import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { TraceMarkers } from './TraceMarkers'
import type { TraceMarker } from './types'

const domain = { min: 0, max: 100_000 }
const marker = (atMs: number, type = 'tool', sessionId = 's1'): TraceMarker => ({
  type,
  at: new Date(atMs).toISOString(),
  session_id: sessionId,
  label: `${type} event`,
})

describe('TraceMarkers', () => {
  it('renders a cluster with count badge and singles without', () => {
    render(
      <div style={{ position: 'relative' }}>
        <TraceMarkers
          markers={[marker(1000), marker(1100), marker(90_000, 'finding')]}
          domain={domain}
          widthPx={1000}
        />
      </div>
    )
    const dots = screen.getAllByTestId('trace-marker')
    expect(dots).toHaveLength(2)
    expect(screen.getByTestId('trace-marker-count')).toHaveTextContent('2')
  })

  it('click calls onSelect with the marker session', () => {
    const onSelect = vi.fn()
    render(
      <TraceMarkers markers={[marker(1000, 'error', 's9')]} domain={domain} widthPx={1000} onSelect={onSelect} />
    )
    fireEvent.click(screen.getByTestId('trace-marker'))
    expect(onSelect).toHaveBeenCalledWith('s9')
  })
})
