import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { WorkflowOriginBadge } from './WorkflowOriginBadge'

const mockUseIsAdmin = vi.fn().mockReturnValue(false)
vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

beforeEach(() => {
  mockUseIsAdmin.mockReset()
  mockUseIsAdmin.mockReturnValue(false)
})

describe('WorkflowOriginBadge', () => {
  it.each([
    ['undefined', undefined],
    ['empty string', ''],
    ['human', 'human'],
  ])('renders nothing when origin is %s', (_label, origin) => {
    const { container } = render(
      <WorkflowOriginBadge origin={origin} originSessionId="sess-1234567890" />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a Console badge with the short session id in the tooltip for origin=console', async () => {
    const user = userEvent.setup()
    render(<WorkflowOriginBadge origin="console" originSessionId="sess-1234567890" />)

    expect(screen.getByText('Console')).toBeInTheDocument()
    await user.hover(screen.getByText('Console'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('Launched from console session sess-123')
  })

  it('renders a plain badge (no link) when useIsAdmin is false', () => {
    mockUseIsAdmin.mockReturnValue(false)
    render(<WorkflowOriginBadge origin="console" originSessionId="sess-1234567890" />)

    expect(screen.getByText('Console')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('renders a /console?session=<id> link when useIsAdmin is true', () => {
    mockUseIsAdmin.mockReturnValue(true)
    render(
      <MemoryRouter>
        <WorkflowOriginBadge origin="console" originSessionId="sess-1234567890" />
      </MemoryRouter>
    )

    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/console?session=sess-1234567890')
  })

  it('renders a plain badge (no link) when admin but no session id is present', () => {
    mockUseIsAdmin.mockReturnValue(true)
    render(<WorkflowOriginBadge origin="console" originSessionId={undefined} />)

    expect(screen.getByText('Console')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
