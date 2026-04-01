import { createRef } from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from './Table'
import { StatusCell } from './StatusCell'

describe('Table', () => {
  it('renders outer div wrapper containing inner table element', () => {
    const { container } = render(<Table><tbody /></Table>)
    const div = container.firstChild as HTMLElement
    expect(div.tagName).toBe('DIV')
    expect(div.querySelector('table')).toBeInTheDocument()
  })

  it('merges className onto outer div', () => {
    const { container } = render(<Table className="my-custom" />)
    expect((container.firstChild as HTMLElement).className).toContain('my-custom')
  })

  it('forwards ref to outer div', () => {
    const ref = createRef<HTMLDivElement>()
    render(<Table ref={ref} />)
    expect(ref.current).toBeInstanceOf(HTMLDivElement)
  })
})

describe('TableHeader', () => {
  it('renders thead, merges className, forwards ref', () => {
    const ref = createRef<HTMLTableSectionElement>()
    const { container } = render(
      <table><TableHeader ref={ref} className="extra-class"><tr><th>H</th></tr></TableHeader></table>
    )
    const thead = container.querySelector('thead') as HTMLElement
    expect(thead).toBeInTheDocument()
    expect(thead.className).toContain('extra-class')
    expect(ref.current?.tagName).toBe('THEAD')
  })
})

describe('TableBody', () => {
  it('renders tbody, merges className, forwards ref', () => {
    const ref = createRef<HTMLTableSectionElement>()
    const { container } = render(<table><TableBody ref={ref} className="my-body" /></table>)
    const tbody = container.querySelector('tbody') as HTMLElement
    expect(tbody).toBeInTheDocument()
    expect(tbody.className).toContain('my-body')
    expect(ref.current?.tagName).toBe('TBODY')
  })
})

describe('TableRow', () => {
  it('renders tr, merges className, forwards ref', () => {
    const ref = createRef<HTMLTableRowElement>()
    const { container } = render(
      <table><tbody><TableRow ref={ref} className="highlight"><td>x</td></TableRow></tbody></table>
    )
    const tr = container.querySelector('tr') as HTMLElement
    expect(tr).toBeInTheDocument()
    expect(tr.className).toContain('highlight')
    expect(ref.current?.tagName).toBe('TR')
  })
})

describe('TableHead', () => {
  it('renders th with children, merges className, forwards ref', () => {
    const ref = createRef<HTMLTableCellElement>()
    const { container } = render(
      <table><thead><tr>
        <TableHead ref={ref} className="min-w-32">Status</TableHead>
      </tr></thead></table>
    )
    const th = container.querySelector('th') as HTMLElement
    expect(th).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
    expect(th.className).toContain('min-w-32')
    expect(ref.current?.tagName).toBe('TH')
  })
})

describe('TableCell', () => {
  it('renders td with children, forwards ref', () => {
    const ref = createRef<HTMLTableCellElement>()
    const { container } = render(
      <table><tbody><tr>
        <TableCell ref={ref}>cell content</TableCell>
      </tr></tbody></table>
    )
    expect(container.querySelector('td')).toBeInTheDocument()
    expect(screen.getByText('cell content')).toBeInTheDocument()
    expect(ref.current?.tagName).toBe('TD')
  })

  it('cn() override — custom className replaces default padding', () => {
    const { container } = render(
      <table><tbody><tr><TableCell className="py-1" /></tr></tbody></table>
    )
    expect((container.querySelector('td') as HTMLElement).className).toContain('py-1')
  })

  it('passes through colSpan', () => {
    const { container } = render(
      <table><tbody><tr><TableCell colSpan={3} /></tr></tbody></table>
    )
    expect((container.querySelector('td') as HTMLTableCellElement).colSpan).toBe(3)
  })
})

describe('StatusCell', () => {
  it('renders status text in a span wrapper', () => {
    render(<StatusCell status="open" />)
    expect(screen.getByText('open')).toBeInTheDocument()
  })

  it.each([
    ['open', 'text-blue-500', false],
    ['in_progress', 'text-yellow-500', true],
    ['running', 'text-yellow-500', true],
    ['closed', 'text-green-500', false],
    ['completed', 'text-green-500', false],
    ['failed', 'text-red-500', false],
    ['error', 'text-red-500', false],
    ['pending', 'text-gray-400', false],
    ['canceled', 'text-orange-500', false],
    ['skipped', 'text-gray-400', false],
    ['unknown_xyz', 'text-gray-400', false],
  ] as const)('status=%s → icon color=%s animate-pulse=%s', (status, expectedColor, pulsed) => {
    const { container } = render(<StatusCell status={status} />)
    const svg = container.querySelector('svg') as SVGElement
    expect(svg).toBeInTheDocument()
    expect(svg.getAttribute('class')).toContain(expectedColor)
    if (pulsed) {
      expect(svg.getAttribute('class')).toContain('animate-pulse')
    } else {
      expect(svg.getAttribute('class')).not.toContain('animate-pulse')
    }
  })

  it('icon has size classes h-4 w-4', () => {
    const { container } = render(<StatusCell status="pending" />)
    const svg = container.querySelector('svg') as SVGElement
    expect(svg.getAttribute('class')).toContain('h-4')
    expect(svg.getAttribute('class')).toContain('w-4')
  })

  it('merges className onto wrapper span', () => {
    const { container } = render(<StatusCell status="open" className="font-bold" />)
    expect((container.firstChild as HTMLElement).className).toContain('font-bold')
  })
})
