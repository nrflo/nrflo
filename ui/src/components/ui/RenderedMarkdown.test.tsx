import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RenderedMarkdown } from './RenderedMarkdown'

describe('RenderedMarkdown', () => {
  it('renders h1 heading', () => {
    render(<RenderedMarkdown content="# Heading One" />)
    expect(screen.getByRole('heading', { level: 1, name: 'Heading One' })).toBeInTheDocument()
  })

  it('renders h2 heading', () => {
    render(<RenderedMarkdown content="## Heading Two" />)
    expect(screen.getByRole('heading', { level: 2, name: 'Heading Two' })).toBeInTheDocument()
  })

  it('renders h3 heading', () => {
    render(<RenderedMarkdown content="### Heading Three" />)
    expect(screen.getByRole('heading', { level: 3, name: 'Heading Three' })).toBeInTheDocument()
  })

  it('renders unordered list items', () => {
    render(<RenderedMarkdown content={`- Apple\n- Banana\n- Cherry`} />)
    expect(screen.getByRole('list')).toBeInTheDocument()
    expect(screen.getByText('Apple')).toBeInTheDocument()
    expect(screen.getByText('Banana')).toBeInTheDocument()
  })

  it('renders ordered list items', () => {
    render(<RenderedMarkdown content={`1. First\n2. Second`} />)
    const list = screen.getByRole('list')
    expect(list.tagName).toBe('OL')
    expect(screen.getByText('First')).toBeInTheDocument()
  })

  it('renders inline code', () => {
    const { container } = render(<RenderedMarkdown content="Use `foo()` here" />)
    const code = container.querySelector('code')
    expect(code).toBeInTheDocument()
    expect(code?.textContent).toBe('foo()')
  })

  it('renders fenced code block', () => {
    const { container } = render(<RenderedMarkdown content={"```js\nconst x = 1\n```"} />)
    const code = container.querySelector('pre code')
    expect(code).toBeInTheDocument()
    expect(code?.textContent).toContain('const x = 1')
  })

  it('renders link with target=_blank', () => {
    render(<RenderedMarkdown content="[Click here](https://example.com)" />)
    const link = screen.getByRole('link', { name: 'Click here' })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', 'https://example.com')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('renders blockquote', () => {
    const { container } = render(<RenderedMarkdown content="> A quoted line" />)
    const blockquote = container.querySelector('blockquote')
    expect(blockquote).toBeInTheDocument()
    expect(blockquote?.textContent).toContain('A quoted line')
  })

  it('renders plain text unchanged', () => {
    render(<RenderedMarkdown content="Just plain text here" />)
    expect(screen.getByText('Just plain text here')).toBeInTheDocument()
  })

  it('renders empty content without error', () => {
    const { container } = render(<RenderedMarkdown content="" />)
    expect(container.querySelector('.markdown-content')).toBeInTheDocument()
  })

  it('wraps output in markdown-content div', () => {
    const { container } = render(<RenderedMarkdown content="Hello" />)
    expect(container.querySelector('.markdown-content')).toBeInTheDocument()
  })
})
