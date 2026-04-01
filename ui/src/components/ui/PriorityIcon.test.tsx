import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PriorityIcon } from './PriorityIcon'

describe('PriorityIcon', () => {
  it.each([
    [1, 'Critical', 'text-red-500'],
    [2, 'High', 'text-orange-500'],
    [3, 'Medium', 'text-yellow-500'],
    [4, 'Low', 'text-blue-500'],
  ] as const)('priority=%d → label=%s color=%s', (priority, label, color) => {
    const { container } = render(<PriorityIcon priority={priority} />)
    expect(screen.getByText(label)).toBeInTheDocument()
    const svg = container.querySelector('svg') as SVGElement
    expect(svg).toBeInTheDocument()
    expect(svg.getAttribute('class')).toContain(color)
    expect(svg.getAttribute('class')).toContain('h-4')
    expect(svg.getAttribute('class')).toContain('w-4')
  })

  it('unknown priority renders fallback icon with gray color and Pn label', () => {
    const { container } = render(<PriorityIcon priority={99} />)
    expect(screen.getByText('P99')).toBeInTheDocument()
    const svg = container.querySelector('svg') as SVGElement
    expect(svg.getAttribute('class')).toContain('text-gray-400')
  })

  it('showLabel=false hides the text label', () => {
    render(<PriorityIcon priority={1} showLabel={false} />)
    expect(screen.queryByText('Critical')).not.toBeInTheDocument()
  })

  it('showLabel defaults to true', () => {
    render(<PriorityIcon priority={2} />)
    expect(screen.getByText('High')).toBeInTheDocument()
  })

  it('passes className to icon', () => {
    const { container } = render(<PriorityIcon priority={3} className="extra-class" />)
    const svg = container.querySelector('svg') as SVGElement
    expect(svg.getAttribute('class')).toContain('extra-class')
  })
})
