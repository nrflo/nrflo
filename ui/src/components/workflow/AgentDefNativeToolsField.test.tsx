import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefNativeToolsField, NATIVE_TOOLS_NONE } from './AgentDefNativeToolsField'

describe('AgentDefNativeToolsField', () => {
  it('default empty value = all tools: chips shown selected and disabled', () => {
    render(<AgentDefNativeToolsField value="" onChange={vi.fn()} />)
    const read = screen.getByRole('button', { name: 'Read' })
    expect(read).toBeDisabled()
  })

  it('None toggle emits the none sentinel', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<AgentDefNativeToolsField value="" onChange={onChange} />)
    await user.click(screen.getByText('None (MCP only)'))
    expect(onChange).toHaveBeenCalledWith(NATIVE_TOOLS_NONE)
  })

  it('toggling a chip from a subset emits a sorted CSV without the sentinel', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<AgentDefNativeToolsField value="Read" onChange={onChange} />)
    await user.click(screen.getByRole('button', { name: 'Edit' }))
    expect(onChange).toHaveBeenCalledWith('Edit,Read')
  })

  it('selecting a chip while none-sentinel active starts a fresh subset', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<AgentDefNativeToolsField value={NATIVE_TOOLS_NONE} onChange={onChange} />)
    await user.click(screen.getByRole('button', { name: 'Read' }))
    expect(onChange).toHaveBeenCalledWith('Read')
  })

  it('warns when a custom subset is empty', () => {
    render(<AgentDefNativeToolsField value=" , " onChange={vi.fn()} />)
    expect(screen.getByText(/No tools selected/)).toBeInTheDocument()
  })

  it('advanced mode exposes the raw CSV', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<AgentDefNativeToolsField value="Read,Grep" onChange={onChange} />)
    await user.click(screen.getByText('Advanced (raw)'))
    const textarea = screen.getByRole('textbox')
    expect((textarea as HTMLTextAreaElement).value).toBe('Read,Grep')
  })
})
