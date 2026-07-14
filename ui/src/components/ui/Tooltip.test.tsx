import { describe, it, expect } from 'vitest'
import type { ComponentProps } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Tooltip } from './Tooltip'

function TestTooltip(props: ComponentProps<typeof Tooltip>) {
  return <Tooltip {...props} delayDuration={0} />
}

describe('Tooltip', () => {
  it('renders children without tooltip initially', () => {
    render(
      <TestTooltip text="Hover me">
        <button>Click me</button>
      </TestTooltip>
    )

    expect(screen.getByText('Click me')).toBeInTheDocument()
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  it('shows tooltip on mouse enter', async () => {
    const user = userEvent.setup()
    render(
      <TestTooltip text="Tooltip text">
        <button>Trigger</button>
      </TestTooltip>
    )

    await user.hover(screen.getByText('Trigger'))
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Tooltip text')
  })

  it('tooltip is accessible via role', async () => {
    const user = userEvent.setup()
    render(
      <TestTooltip text="Tooltip text">
        <button>Trigger</button>
      </TestTooltip>
    )

    // Not visible initially
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()

    // Shows on hover with proper ARIA role
    await user.hover(screen.getByText('Trigger'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('Tooltip text')

    // Trigger is linked via aria-describedby
    const trigger = screen.getByText('Trigger').closest('[aria-describedby]')
    expect(trigger).toBeInTheDocument()
  })

  it('supports different placement options', async () => {
    const user = userEvent.setup()
    const placements: Array<'top' | 'bottom' | 'left' | 'right'> = ['top', 'bottom', 'left', 'right']

    for (const placement of placements) {
      const { unmount } = render(
        <TestTooltip text={`${placement} tooltip`} placement={placement}>
          <button>{placement}</button>
        </TestTooltip>
      )

      await user.hover(screen.getByText(placement))
      const tooltip = await screen.findByRole('tooltip')
      expect(tooltip).toHaveTextContent(`${placement} tooltip`)
      unmount()
    }
  })

  it('applies custom className', async () => {
    const user = userEvent.setup()
    render(
      <TestTooltip text="Custom" className="custom-class">
        <button>Trigger</button>
      </TestTooltip>
    )

    await user.hover(screen.getByText('Trigger'))
    await screen.findByRole('tooltip')
    // className is applied to the RadixTooltip.Content element
    const contentEl = document.querySelector('[data-side]')
    expect(contentEl).toHaveClass('custom-class')
  })

  it('renders tooltip in document.body via portal', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <div data-testid="wrapper">
        <TestTooltip text="Portal test">
          <button>Trigger</button>
        </TestTooltip>
      </div>
    )

    await user.hover(screen.getByText('Trigger'))
    const tooltip = await screen.findByRole('tooltip')

    // Tooltip should be in document.body, not in the wrapper
    expect(container.querySelector('[data-testid="wrapper"]')).not.toContainElement(tooltip)
    expect(document.body).toContainElement(tooltip)
  })

  it('renders ReactNode JSX content in tooltip', async () => {
    const user = userEvent.setup()
    render(
      <TestTooltip text={<><p>First paragraph</p><p>Second paragraph</p></>}>
        <button>Trigger</button>
      </TestTooltip>
    )

    await user.hover(screen.getByText('Trigger'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('First paragraph')
    expect(tooltip).toHaveTextContent('Second paragraph')
  })

  it('shows tooltip on keyboard focus', async () => {
    const user = userEvent.setup()
    render(
      <TestTooltip text="Focus tooltip">
        <button>Trigger</button>
      </TestTooltip>
    )

    await user.tab()
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Focus tooltip')
  })
})
