import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ForbiddenPage } from './ForbiddenPage'

describe('ForbiddenPage', () => {
  it('renders 403 heading', () => {
    render(
      <MemoryRouter>
        <ForbiddenPage />
      </MemoryRouter>
    )
    expect(screen.getByText('403')).toBeInTheDocument()
  })

  it('renders permission error message', () => {
    render(
      <MemoryRouter>
        <ForbiddenPage />
      </MemoryRouter>
    )
    expect(
      screen.getByText("You don't have permission to access this page.")
    ).toBeInTheDocument()
  })

  it('renders link to home page', () => {
    render(
      <MemoryRouter>
        <ForbiddenPage />
      </MemoryRouter>
    )
    const link = screen.getByRole('link', { name: /go back home/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/')
  })
})
