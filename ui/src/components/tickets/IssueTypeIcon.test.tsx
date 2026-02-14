import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { IssueTypeIcon } from './IssueTypeIcon'

describe('IssueTypeIcon', () => {
  it('renders bug icon with red color', () => {
    const { container } = render(<IssueTypeIcon type="bug" />)
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveClass('text-red-500')
    expect(svg).toHaveClass('h-4')
    expect(svg).toHaveClass('w-4')
  })

  it('renders feature icon with purple color', () => {
    const { container } = render(<IssueTypeIcon type="feature" />)
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveClass('text-purple-500')
  })

  it('renders task icon with blue color', () => {
    const { container } = render(<IssueTypeIcon type="task" />)
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveClass('text-blue-500')
  })

  it('renders epic icon with green color', () => {
    const { container } = render(<IssueTypeIcon type="epic" />)
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveClass('text-green-500')
  })

  it('renders default icon with gray color for unknown type', () => {
    const { container } = render(<IssueTypeIcon type="unknown" />)
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveClass('text-gray-500')
  })

  it('renders small size by default', () => {
    const { container } = render(<IssueTypeIcon type="bug" />)
    const svg = container.querySelector('svg')
    expect(svg).toHaveClass('h-4')
    expect(svg).toHaveClass('w-4')
  })

  it('renders medium size when specified', () => {
    const { container } = render(<IssueTypeIcon type="feature" size="md" />)
    const svg = container.querySelector('svg')
    expect(svg).toHaveClass('h-5')
    expect(svg).toHaveClass('w-5')
  })

  it('renders small size when explicitly specified', () => {
    const { container } = render(<IssueTypeIcon type="task" size="sm" />)
    const svg = container.querySelector('svg')
    expect(svg).toHaveClass('h-4')
    expect(svg).toHaveClass('w-4')
  })
})
