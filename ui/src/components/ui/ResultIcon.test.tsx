import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ResultIcon } from './ResultIcon'

describe('ResultIcon', () => {
  it.each([
    ['pass', 'pass', 'text-green-500'],
    ['fail', 'fail', 'text-red-500'],
    ['skip', 'skipped', 'text-gray-400'],
    ['skipped', 'skipped', 'text-gray-400'],
  ] as const)('result=%s → text=%s color=%s', (result, text, color) => {
    const { container } = render(<ResultIcon result={result} />)
    expect(screen.getByText(text)).toBeInTheDocument()
    const svg = container.querySelector('svg') as SVGElement
    expect(svg).toBeInTheDocument()
    expect(svg.getAttribute('class')).toContain(color)
  })

  it('unknown result renders fallback with result text and gray icon', () => {
    const { container } = render(<ResultIcon result="continue" />)
    expect(screen.getByText('continue')).toBeInTheDocument()
    const svg = container.querySelector('svg') as SVGElement
    expect(svg.getAttribute('class')).toContain('text-gray-400')
  })

  it('icon has size classes h-4 w-4', () => {
    const { container } = render(<ResultIcon result="pass" />)
    const svg = container.querySelector('svg') as SVGElement
    expect(svg.getAttribute('class')).toContain('h-4')
    expect(svg.getAttribute('class')).toContain('w-4')
  })

  it('passes className to icon', () => {
    const { container } = render(<ResultIcon result="pass" className="custom-size" />)
    const svg = container.querySelector('svg') as SVGElement
    expect(svg.getAttribute('class')).toContain('custom-size')
  })
})
