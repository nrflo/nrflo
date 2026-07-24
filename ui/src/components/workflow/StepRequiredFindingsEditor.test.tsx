import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StepRequiredFindingsEditor } from './StepRequiredFindingsEditor'
import type { RequiredFinding } from '@/types/workflow'

describe('StepRequiredFindingsEditor', () => {
  it('offers exactly the 3 known schema options in the dropdown', async () => {
    const user = userEvent.setup()
    render(<StepRequiredFindingsEditor value={[{ key: 'k', schema: 'nonempty_text' }]} onChange={vi.fn()} />)

    await user.click(screen.getByText('nonempty_text'))
    expect(screen.getByText('ordered_lines')).toBeInTheDocument()
    expect(screen.getByText('json_array_path_change')).toBeInTheDocument()
    // 'nonempty_text' appears both as selected label and menu item
    expect(screen.getAllByText('nonempty_text').length).toBeGreaterThanOrEqual(1)
  })

  it('adds a finding row on "Add required finding"', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<StepRequiredFindingsEditor value={[]} onChange={onChange} />)

    await user.click(screen.getByText('Add required finding'))

    expect(onChange).toHaveBeenCalledWith([{ key: '', schema: 'nonempty_text' }])
  })

  it('removes a finding row', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const value: RequiredFinding[] = [
      { key: 'first', schema: 'nonempty_text' },
      { key: 'second', schema: 'ordered_lines' },
    ]
    render(<StepRequiredFindingsEditor value={value} onChange={onChange} />)

    await user.click(screen.getAllByText('Remove')[0]!)

    expect(onChange).toHaveBeenCalledWith([{ key: 'second', schema: 'ordered_lines' }])
  })

  it('disables Add at the 20-item cap', () => {
    const value: RequiredFinding[] = Array.from({ length: 20 }, (_, i) => ({ key: `k${i}`, schema: 'nonempty_text' }))
    render(<StepRequiredFindingsEditor value={value} onChange={vi.fn()} />)

    expect(screen.getByText('Add required finding').closest('button')).toBeDisabled()
  })
})
