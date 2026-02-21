import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Dropdown } from './Dropdown'

const OPTIONS = [
  { value: 'alpha', label: 'Alpha' },
  { value: 'beta', label: 'Beta' },
  { value: 'gamma', label: 'Gamma' },
]

function renderDropdown(props: Partial<React.ComponentProps<typeof Dropdown>> = {}) {
  const onChange = props.onChange ?? vi.fn()
  render(
    <Dropdown
      value={props.value ?? ''}
      onChange={onChange}
      options={props.options ?? OPTIONS}
      {...props}
    />
  )
  return { onChange }
}

describe('Dropdown', () => {
  it('renders trigger with placeholder when no value selected', () => {
    renderDropdown({ placeholder: 'Pick one' })
    expect(screen.getByText('Pick one')).toBeInTheDocument()
    expect(screen.queryByRole('option')).not.toBeInTheDocument()
  })

  it('renders selected option label in trigger', () => {
    renderDropdown({ value: 'beta' })
    expect(screen.getByText('Beta')).toBeInTheDocument()
  })

  it('opens panel with all options on trigger click', async () => {
    const user = userEvent.setup()
    renderDropdown()

    await user.click(screen.getByRole('button'))

    expect(screen.getByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('Beta')).toBeInTheDocument()
    expect(screen.getByText('Gamma')).toBeInTheDocument()
  })

  it('closes panel on second trigger click', async () => {
    const user = userEvent.setup()
    renderDropdown()
    const btn = screen.getByRole('button')

    await user.click(btn)
    expect(screen.getByText('Alpha')).toBeInTheDocument()

    await user.click(btn)
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  })

  it('calls onChange with selected value and closes panel', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    renderDropdown({ onChange })

    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('Beta'))

    expect(onChange).toHaveBeenCalledOnce()
    expect(onChange).toHaveBeenCalledWith('beta')
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  })

  it('shows Check icon visible only on selected option', async () => {
    const user = userEvent.setup()
    renderDropdown({ value: 'alpha' })

    await user.click(screen.getByRole('button'))

    // Panel option spans have className="truncate"
    const allAlpha = screen.getAllByText('Alpha')
    const alphaOptionSpan = allAlpha.find((el) => el.className === 'truncate')!
    const alphaOptionDiv = alphaOptionSpan.parentElement!
    const betaOptionSpan = screen.getByText('Beta') // only one Beta element
    const betaOptionDiv = betaOptionSpan.parentElement!

    // Selected option class includes 'bg-muted text-foreground' (exact sequence)
    expect(alphaOptionDiv.className).toContain('bg-muted text-foreground')
    // Unselected does not have 'bg-muted text-foreground' (only hover:bg-muted)
    expect(betaOptionDiv.className).not.toContain('bg-muted text-foreground')
  })

  it('closes on Escape key', async () => {
    const user = userEvent.setup()
    renderDropdown()

    await user.click(screen.getByRole('button'))
    expect(screen.getByText('Alpha')).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  })

  it('closes on click outside', async () => {
    const user = userEvent.setup()
    render(
      <div>
        <button data-testid="outside">outside</button>
        <Dropdown value="" onChange={vi.fn()} options={OPTIONS} />
      </div>
    )

    await user.click(screen.getByText('Select...').closest('button')!)
    expect(screen.getByText('Alpha')).toBeInTheDocument()

    await user.click(screen.getByTestId('outside'))
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  })

  it('does not open when disabled', async () => {
    const user = userEvent.setup()
    renderDropdown({ disabled: true })

    await user.click(screen.getByRole('button'))
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  })

  it('applies disabled styling when disabled', () => {
    renderDropdown({ disabled: true })
    const btn = screen.getByRole('button')
    expect(btn.className).toContain('opacity-50')
    expect(btn.className).toContain('cursor-not-allowed')
  })

  it('renders custom icon in trigger', () => {
    render(
      <Dropdown
        value=""
        onChange={vi.fn()}
        options={OPTIONS}
        icon={<span data-testid="custom-icon">★</span>}
      />
    )
    expect(screen.getByTestId('custom-icon')).toBeInTheDocument()
  })

  it('applies custom className to trigger button', () => {
    renderDropdown({ className: 'w-auto' })
    expect(screen.getByRole('button').className).toContain('w-auto')
  })

  it('uses default placeholder when none provided', () => {
    renderDropdown({ value: '' })
    expect(screen.getByText('Select...')).toBeInTheDocument()
  })

  it('ProjectSelect renders with FolderOpen icon via Dropdown', async () => {
    const { ProjectSelect } = await import('./ProjectSelect')
    const onChange = vi.fn()
    const projects = [
      { id: 'p1', name: 'Project One' },
      { id: 'p2', name: 'Project Two' },
    ]
    render(<ProjectSelect value="" onChange={onChange} projects={projects} />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button'))

    expect(screen.getByText('Project One')).toBeInTheDocument()
    expect(screen.getByText('Project Two')).toBeInTheDocument()

    await user.click(screen.getByText('Project One'))
    expect(onChange).toHaveBeenCalledWith('p1')
  })

  it('applies labelClassName to the label span', () => {
    renderDropdown({ value: 'alpha', labelClassName: 'hidden md:inline' })
    const btn = screen.getByRole('button')
    // The label span is inside the button, has the truncate + labelClassName classes
    const labelSpan = btn.querySelector('span.hidden')
    expect(labelSpan).toBeInTheDocument()
    expect(labelSpan).toHaveClass('hidden', 'md:inline')
  })

  it('ProjectSelect applies hidden md:inline labelClassName for responsive label hiding', async () => {
    const { ProjectSelect } = await import('./ProjectSelect')
    const projects = [{ id: 'p1', name: 'Alpha Project' }]
    render(<ProjectSelect value="p1" onChange={vi.fn()} projects={projects} />)

    const btn = screen.getByRole('button')
    const labelSpan = btn.querySelector('span.hidden')
    expect(labelSpan).toBeInTheDocument()
    expect(labelSpan).toHaveClass('hidden', 'md:inline')
    expect(labelSpan).toHaveTextContent('Alpha Project')
  })
})
