import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Tooltip } from './Tooltip'

describe('Tooltip', () => {
  it('renders children without tooltip initially', () => {
    render(
      <Tooltip text="Hover me">
        <button>Click me</button>
      </Tooltip>
    )

    expect(screen.getByText('Click me')).toBeInTheDocument()
    expect(screen.queryByText('Hover me')).not.toBeInTheDocument()
  })

  it('shows tooltip on mouse enter', async () => {
    const user = userEvent.setup()
    render(
      <Tooltip text="Tooltip text">
        <button>Trigger</button>
      </Tooltip>
    )

    await user.hover(screen.getByText('Trigger'))
    expect(screen.getByText('Tooltip text')).toBeInTheDocument()
  })

  it('hides tooltip on mouse leave', async () => {
    const user = userEvent.setup()
    render(
      <Tooltip text="Tooltip text">
        <button>Trigger</button>
      </Tooltip>
    )

    const trigger = screen.getByText('Trigger')
    await user.hover(trigger)
    expect(screen.getByText('Tooltip text')).toBeInTheDocument()

    await user.unhover(trigger)
    expect(screen.queryByText('Tooltip text')).not.toBeInTheDocument()
  })

  it('applies correct placement styles', async () => {
    const user = userEvent.setup()
    render(
      <Tooltip text="Top tooltip" placement="top">
        <button>Trigger</button>
      </Tooltip>
    )

    await user.hover(screen.getByText('Trigger'))
    const tooltip = document.body.querySelector('.fixed')
    expect(tooltip).toBeInTheDocument()
    expect(tooltip?.className).toContain('-translate-x-1/2')
    expect(tooltip?.className).toContain('-translate-y-full')
  })

  it('supports different placement options', async () => {
    const user = userEvent.setup()
    const placements: Array<'top' | 'bottom' | 'left' | 'right'> = ['top', 'bottom', 'left', 'right']

    for (const placement of placements) {
      const { container, unmount } = render(
        <Tooltip text={`${placement} tooltip`} placement={placement}>
          <button>{placement}</button>
        </Tooltip>
      )

      await user.hover(screen.getByText(placement))
      expect(screen.getByText(`${placement} tooltip`)).toBeInTheDocument()
      unmount()
    }
  })

  it('applies custom className', async () => {
    const user = userEvent.setup()
    render(
      <Tooltip text="Custom" className="custom-class">
        <button>Trigger</button>
      </Tooltip>
    )

    await user.hover(screen.getByText('Trigger'))
    const tooltip = document.body.querySelector('.custom-class')
    expect(tooltip).toBeInTheDocument()
  })

  it('renders tooltip in document.body via portal', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <div data-testid="wrapper">
        <Tooltip text="Portal test">
          <button>Trigger</button>
        </Tooltip>
      </div>
    )

    await user.hover(screen.getByText('Trigger'))
    const tooltip = screen.getByText('Portal test')

    // Tooltip should be in document.body, not in the wrapper
    expect(container.querySelector('[data-testid="wrapper"]')).not.toContainElement(tooltip)
    expect(document.body).toContainElement(tooltip)
  })
})
