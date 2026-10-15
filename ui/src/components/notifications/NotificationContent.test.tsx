import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { NotificationContent } from './NotificationContent'

describe('NotificationContent', () => {
  it('renders Markdown with headings, lists, links, and code', () => {
    const { container } = render(<NotificationContent value={'## Result\n\n**Done**\n\n- First\n- Second\n\n[Details](https://example.com)\n\n```go\nfmt.Println("done")\n```'} />)
    expect(screen.getByRole('heading', { name: 'Result' })).toBeInTheDocument()
    expect(container.querySelector('strong')).toHaveTextContent('Done')
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByRole('link', { name: 'Details' })).toHaveAttribute('rel', 'noopener noreferrer')
    expect(container.querySelector('pre code')).toHaveTextContent('fmt.Println')
  })

  it.each([{ status: 'done', items: [1, 2] }, '{"status":"done","items":[1,2]}'])(
    'pretty prints structured and serialized JSON', (value) => {
      const { container } = render(<NotificationContent value={value} />)
      expect(container.querySelector('pre code')?.textContent).toBe(JSON.stringify({ status: 'done', items: [1, 2] }, null, 2))
    },
  )

  it('keeps untrusted HTML inert', () => {
    const { container } = render(<NotificationContent value={'<script>alert(1)</script>\n\n[bad](javascript:alert(1))'} />)
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('a')).not.toHaveAttribute('href', 'javascript:alert(1)')
  })
})
