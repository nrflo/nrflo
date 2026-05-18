import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useState } from 'react'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectArtifactStorageEditor, type FormState } from './ProjectArtifactStorageEditor'
import { renderWithQuery } from '@/test/utils'

const PROJECT_ID = 'proj-1'

beforeEach(() => vi.clearAllMocks())

const defaultForm: FormState = {
  mode: 'internal',
  account_id: '',
  bucket: '',
  prefix: '',
  access_key_ref: '',
  secret_key_ref: '',
}

function StatefulArtifactEditor({
  onChange,
  initialValue,
}: {
  onChange: (v: FormState) => void
  initialValue: FormState
}) {
  const [value, setValue] = useState(initialValue)
  return (
    <ProjectArtifactStorageEditor
      projectId={PROJECT_ID}
      value={value}
      onChange={(next) => { setValue(next); onChange(next) }}
    />
  )
}

describe('ProjectArtifactStorageEditor', () => {
  it('renders mode=internal from value prop', () => {
    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} value={defaultForm} onChange={vi.fn()} />)
    expect(screen.getByText('Internal')).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('your-account-id')).not.toBeInTheDocument()
  })

  it('value prop with cloudflare_r2 reveals R2 fields', () => {
    const r2Value: FormState = { ...defaultForm, mode: 'cloudflare_r2' }
    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} value={r2Value} onChange={vi.fn()} />)
    expect(screen.getByPlaceholderText('your-account-id')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('my-bucket')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('optional/path/prefix/')).toBeInTheDocument()
  })

  it('switching to Cloudflare R2 in dropdown calls onChange with cloudflare_r2 mode', async () => {
    const onChange = vi.fn()
    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} value={defaultForm} onChange={onChange} />)

    const user = userEvent.setup()
    const dropdownTrigger = screen.getByRole('button', { name: /internal/i })
    await user.click(dropdownTrigger)

    const r2Option = screen.getByText('Cloudflare R2')
    await user.click(r2Option)

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ mode: 'cloudflare_r2' }))
  })

  it('masked secrets (literal:***) from value prop are displayed in inputs', () => {
    const maskedValue: FormState = {
      ...defaultForm,
      mode: 'cloudflare_r2',
      account_id: 'acct123',
      bucket: 'mybucket',
      access_key_ref: 'literal:***',
      secret_key_ref: 'literal:***',
    }
    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} value={maskedValue} onChange={vi.fn()} />)

    const keyInputs = screen.getAllByDisplayValue('literal:***')
    expect(keyInputs).toHaveLength(2)
  })

  it('typing a new secret calls onChange with updated access_key_ref', async () => {
    const onChange = vi.fn()
    const maskedValue: FormState = { ...defaultForm, mode: 'cloudflare_r2', access_key_ref: 'literal:***' }
    renderWithQuery(<StatefulArtifactEditor initialValue={maskedValue} onChange={onChange} />)

    const user = userEvent.setup()
    const accessKeyInput = screen.getByPlaceholderText('env:R2_ACCESS_KEY')
    await user.clear(accessKeyInput)
    await user.type(accessKeyInput, 'env:NEW_KEY')

    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ access_key_ref: 'env:NEW_KEY' }))
  })

  it('server error is rendered verbatim', () => {
    renderWithQuery(
      <ProjectArtifactStorageEditor
        projectId={PROJECT_ID}
        value={defaultForm}
        onChange={vi.fn()}
        serverError="s3 backend not yet implemented"
      />
    )
    expect(screen.getByText('s3 backend not yet implemented')).toBeInTheDocument()
  })

  it('serverError not rendered when prop is absent', () => {
    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} value={defaultForm} onChange={vi.fn()} />)
    expect(screen.queryByRole('paragraph')).not.toBeInTheDocument()
  })
})
