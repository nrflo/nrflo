import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ConfigEditorPage } from './ConfigEditor'
import type { NrvappConfigFile, NrvappConfigVersion } from '@/types/nrvapp'

vi.mock('@/hooks/useNrvapp', () => ({
  useConfigFile: vi.fn(),
  useConfigHistory: vi.fn(),
  usePutConfigFile: vi.fn(),
  useRollbackConfig: vi.fn(),
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({
    value,
    onChange,
  }: {
    value: string
    onChange?: (v: string) => void
  }) => (
    <textarea
      data-testid="markdown-editor"
      defaultValue={value}
      onChange={(e) => onChange?.(e.target.value)}
    />
  ),
}))

vi.mock('@/components/nrvapp/VersionHistory', () => ({
  VersionHistory: ({
    versions,
    onRollback,
  }: {
    versions: NrvappConfigVersion[]
    currentVersion: number
    onRollback: (v: number) => void
    isRollingBack: boolean
  }) => (
    <div data-testid="version-history">
      {versions.map((v) => (
        <button
          key={v.version}
          data-testid={`rollback-${v.version}`}
          onClick={() => onRollback(v.version)}
        >
          Rollback to {v.version}
        </button>
      ))}
    </div>
  ),
}))

vi.mock('@/components/nrvapp/DiffPreview', () => ({
  DiffPreview: ({ before, after }: { before: string; after: string }) => (
    <div data-testid="diff-preview" data-before={before} data-after={after} />
  ),
}))

// Mock RJSF to avoid complex schema form in tests
vi.mock('@rjsf/core', () => ({
  default: ({ children, onSubmit }: { children: React.ReactNode; onSubmit: () => void }) => (
    <form data-testid="rjsf-form" onSubmit={(e) => { e.preventDefault(); onSubmit() }}>
      {children}
    </form>
  ),
}))

vi.mock('@rjsf/validator-ajv8', () => ({
  default: {},
}))

import {
  useConfigFile,
  useConfigHistory,
  usePutConfigFile,
  useRollbackConfig,
} from '@/hooks/useNrvapp'

function makeConfigFile(overrides: Partial<NrvappConfigFile> = {}): NrvappConfigFile {
  return {
    path: 'customer/config.yaml',
    content: 'key: value',
    version: 1,
    ...overrides,
  }
}

function makeVersion(version: number): NrvappConfigVersion {
  return {
    version,
    actor: 'user',
    created_at: '2026-01-01T00:00:00Z',
  }
}

const mockPutMutate = vi.fn()
const mockRollbackMutate = vi.fn()

function setupMocks(file = makeConfigFile(), history: NrvappConfigVersion[] = []) {
  vi.mocked(useConfigFile).mockReturnValue({
    data: file,
    isLoading: false,
  } as unknown as ReturnType<typeof useConfigFile>)
  vi.mocked(useConfigHistory).mockReturnValue({
    data: history,
  } as unknown as ReturnType<typeof useConfigHistory>)
  vi.mocked(usePutConfigFile).mockReturnValue({
    mutate: mockPutMutate,
    isPending: false,
  } as unknown as ReturnType<typeof usePutConfigFile>)
  vi.mocked(useRollbackConfig).mockReturnValue({
    mutate: mockRollbackMutate,
    isPending: false,
  } as unknown as ReturnType<typeof useRollbackConfig>)
}

function renderPage(filePath = 'customer%2Fconfig.yaml') {
  return render(
    <MemoryRouter initialEntries={[`/nrvapp/config/${filePath}`]}>
      <Routes>
        <Route path="/nrvapp/config/:file" element={<ConfigEditorPage />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('ConfigEditorPage', () => {
  describe('loading state', () => {
    it('shows loading when file is loading', () => {
      vi.mocked(useConfigFile).mockReturnValue({
        data: undefined,
        isLoading: true,
      } as unknown as ReturnType<typeof useConfigFile>)
      vi.mocked(useConfigHistory).mockReturnValue({
        data: [],
      } as unknown as ReturnType<typeof useConfigHistory>)
      vi.mocked(usePutConfigFile).mockReturnValue({
        mutate: mockPutMutate,
        isPending: false,
      } as unknown as ReturnType<typeof usePutConfigFile>)
      vi.mocked(useRollbackConfig).mockReturnValue({
        mutate: mockRollbackMutate,
        isPending: false,
      } as unknown as ReturnType<typeof useRollbackConfig>)
      renderPage()
      expect(screen.getByText('Loading…')).toBeInTheDocument()
    })
  })

  describe('plain text editor (no schema)', () => {
    it('renders MarkdownEditor when file has no schema', () => {
      setupMocks(makeConfigFile({ schema: undefined }))
      renderPage()
      expect(screen.getByTestId('markdown-editor')).toBeInTheDocument()
    })

    it('Save button calls putConfigFile.mutate with path and content', async () => {
      const user = userEvent.setup()
      setupMocks(makeConfigFile({ path: 'customer/config.yaml', content: 'key: value' }))
      renderPage()
      await user.click(screen.getByRole('button', { name: 'Save' }))
      expect(mockPutMutate).toHaveBeenCalledWith(
        expect.objectContaining({ path: 'customer/config.yaml' })
      )
    })

    it('Save button disabled while isPending', () => {
      setupMocks()
      vi.mocked(usePutConfigFile).mockReturnValue({
        mutate: mockPutMutate,
        isPending: true,
      } as unknown as ReturnType<typeof usePutConfigFile>)
      renderPage()
      expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    })
  })

  describe('schema-driven form', () => {
    it('renders RJSF form when file has schema', () => {
      setupMocks(
        makeConfigFile({
          schema: { type: 'object', properties: { name: { type: 'string' } } },
          content: 'name: test',
        })
      )
      renderPage()
      expect(screen.getByTestId('rjsf-form')).toBeInTheDocument()
    })

    it('does not render MarkdownEditor when schema is present', () => {
      setupMocks(
        makeConfigFile({
          schema: { type: 'object' },
          content: 'key: value',
        })
      )
      renderPage()
      expect(screen.queryByTestId('markdown-editor')).not.toBeInTheDocument()
    })
  })

  describe('version history', () => {
    it('renders VersionHistory with versions', () => {
      setupMocks(makeConfigFile(), [makeVersion(1), makeVersion(2)])
      renderPage()
      expect(screen.getByTestId('version-history')).toBeInTheDocument()
      expect(screen.getByTestId('rollback-1')).toBeInTheDocument()
      expect(screen.getByTestId('rollback-2')).toBeInTheDocument()
    })

    it('rollback button calls rollbackConfig.mutate with path and version', async () => {
      const user = userEvent.setup()
      setupMocks(makeConfigFile({ path: 'customer/config.yaml' }), [makeVersion(2)])
      renderPage()
      await user.click(screen.getByTestId('rollback-2'))
      expect(mockRollbackMutate).toHaveBeenCalledWith(
        { path: 'customer/config.yaml', version: 2 },
        expect.any(Object)
      )
    })

    it('shows diff preview after rollback succeeds', async () => {
      const user = userEvent.setup()
      vi.mocked(useRollbackConfig).mockReturnValue({
        mutate: (vars: unknown, opts: { onSuccess: (data: NrvappConfigFile) => void }) => {
          opts.onSuccess(makeConfigFile({ content: 'new content', version: 2 }))
        },
        isPending: false,
      } as unknown as ReturnType<typeof useRollbackConfig>)
      setupMocks(makeConfigFile(), [makeVersion(2)])
      vi.mocked(useRollbackConfig).mockReturnValue({
        mutate: (vars: unknown, opts: { onSuccess: (data: NrvappConfigFile) => void }) => {
          opts.onSuccess(makeConfigFile({ content: 'new content', version: 2 }))
        },
        isPending: false,
      } as unknown as ReturnType<typeof useRollbackConfig>)
      renderPage()
      await user.click(screen.getByTestId('rollback-2'))
      expect(screen.getByTestId('diff-preview')).toBeInTheDocument()
    })
  })
})
