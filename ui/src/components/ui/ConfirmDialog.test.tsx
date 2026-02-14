import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmDialog } from './ConfirmDialog'

describe('ConfirmDialog', () => {
  it('does not render when open is false', () => {
    render(
      <ConfirmDialog
        open={false}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="Confirm"
        message="Are you sure?"
      />
    )

    expect(screen.queryByText('Confirm')).not.toBeInTheDocument()
    expect(screen.queryByText('Are you sure?')).not.toBeInTheDocument()
  })

  it('renders when open is true', () => {
    render(
      <ConfirmDialog
        open={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="Confirm Action"
        message="Do you want to proceed?"
      />
    )

    expect(screen.getByText('Confirm Action')).toBeInTheDocument()
    expect(screen.getByText('Do you want to proceed?')).toBeInTheDocument()
  })

  it('shows default confirm button label', () => {
    render(
      <ConfirmDialog
        open={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="Test"
        message="Test message"
      />
    )

    expect(screen.getByText('Confirm')).toBeInTheDocument()
    expect(screen.getByText('Cancel')).toBeInTheDocument()
  })

  it('shows custom confirm button label', () => {
    render(
      <ConfirmDialog
        open={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="Delete"
        message="Delete item?"
        confirmLabel="Delete Now"
      />
    )

    expect(screen.getByText('Delete Now')).toBeInTheDocument()
  })

  it('calls onClose when cancel button is clicked', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()

    render(
      <ConfirmDialog
        open={true}
        onClose={onClose}
        onConfirm={vi.fn()}
        title="Test"
        message="Test message"
      />
    )

    await user.click(screen.getByText('Cancel'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('calls onConfirm and onClose when confirm button is clicked', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    const onConfirm = vi.fn()

    render(
      <ConfirmDialog
        open={true}
        onClose={onClose}
        onConfirm={onConfirm}
        title="Test"
        message="Test message"
      />
    )

    await user.click(screen.getByText('Confirm'))
    expect(onConfirm).toHaveBeenCalledTimes(1)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('applies default variant styling', () => {
    const { container } = render(
      <ConfirmDialog
        open={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="Test"
        message="Test"
      />
    )

    // Default variant should not have destructive styling
    const confirmButton = screen.getByText('Confirm')
    expect(confirmButton.className).not.toContain('destructive')
  })

  it('applies destructive variant styling', () => {
    render(
      <ConfirmDialog
        open={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="Delete"
        message="Delete?"
        variant="destructive"
      />
    )

    const confirmButton = screen.getByText('Confirm')
    expect(confirmButton.className).toContain('destructive')
  })

  it('calls onClose when close icon is clicked', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()

    render(
      <ConfirmDialog
        open={true}
        onClose={onClose}
        onConfirm={vi.fn()}
        title="Test"
        message="Test"
      />
    )

    // Find close button (X icon) in header - it's the button that contains the X icon
    // There are 3 buttons: close (X), Cancel, Confirm
    const buttons = screen.getAllByRole('button')
    // First button should be the X close button in the header
    await user.click(buttons[0])
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('does not call onConfirm when cancel is clicked', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()

    render(
      <ConfirmDialog
        open={true}
        onClose={vi.fn()}
        onConfirm={onConfirm}
        title="Test"
        message="Test"
      />
    )

    await user.click(screen.getByText('Cancel'))
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('handles ESC key press to close', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()

    render(
      <ConfirmDialog
        open={true}
        onClose={onClose}
        onConfirm={vi.fn()}
        title="Test"
        message="Test"
      />
    )

    await user.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
