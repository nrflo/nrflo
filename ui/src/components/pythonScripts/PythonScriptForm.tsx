import { useState } from 'react'
import { CheckCircle, AlertCircle, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { useValidatePythonScript, useReadPythonFile } from '@/hooks/usePythonScripts'
import { FilePickerModal } from './FilePickerModal'
import type { PythonScript, PythonScriptCreateRequest, PythonScriptUpdateRequest, ValidationResult } from '@/types/pythonScript'

export type FormData = PythonScriptCreateRequest | PythonScriptUpdateRequest

interface PythonScriptFormProps {
  initial?: PythonScript
  isCreate: boolean
  onSubmit: (data: FormData) => void
  onValidationFailure: (result: ValidationResult, data: FormData) => void
  onCancel: () => void
  isPending?: boolean
}

export function PythonScriptForm({
  initial,
  isCreate,
  onSubmit,
  onValidationFailure,
  onCancel,
  isPending,
}: PythonScriptFormProps) {
  const [name, setName] = useState(initial?.name || '')
  const [description, setDescription] = useState(initial?.description || '')
  const [code, setCode] = useState(initial?.code || '')
  const [syntaxResult, setSyntaxResult] = useState<ValidationResult | null>(null)
  const [filePath, setFilePath] = useState(initial?.file_path || '')
  const [modalOpen, setModalOpen] = useState(false)

  const validateMutation = useValidatePythonScript()
  const fileQuery = useReadPythonFile(filePath || null)

  const handleCheckSyntax = async () => {
    setSyntaxResult(null)
    const result = await validateMutation.mutateAsync(code)
    setSyntaxResult(result)
  }

  const buildFormData = (): FormData =>
    isCreate
      ? ({ name, description: description || undefined, code, file_path: filePath } as PythonScriptCreateRequest)
      : ({ name, description: description || undefined, code, file_path: filePath } as PythonScriptUpdateRequest)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return

    if (filePath) {
      onSubmit(buildFormData())
      return
    }

    const result = await validateMutation.mutateAsync(code)
    setSyntaxResult(result)
    const data = buildFormData()

    if (!result.ok) {
      onValidationFailure(result, data)
      return
    }

    onSubmit(data)
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Name</label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g., data-processor"
          required
        />
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Description</label>
        <Textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="What does this script do?"
          rows={2}
        />
      </div>
      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="text-xs font-medium text-muted-foreground">File path</label>
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setModalOpen(true)}>
              Browse…
            </Button>
            {filePath && (
              <Button type="button" variant="ghost" size="sm" onClick={() => setFilePath('')}>
                Clear
              </Button>
            )}
          </div>
        </div>
        <Input
          value={filePath}
          disabled
          placeholder="(none — using template below)"
        />
      </div>
      {filePath ? (
        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="text-xs font-medium text-muted-foreground">Code (read from file)</label>
            <div className="flex items-center gap-2">
              {fileQuery.isError && (
                <span className="flex items-center gap-1 text-xs text-destructive">
                  <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                  Failed to read file
                </span>
              )}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => fileQuery.refetch()}
                disabled={fileQuery.isFetching}
              >
                {fileQuery.isFetching && <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" />}
                Reload from file
              </Button>
            </div>
          </div>
          <CodeEditor
            value={fileQuery.data?.content || ''}
            onChange={() => {}}
            language="python"
            readOnly
            minHeight="240px"
            maxHeight="500px"
          />
        </div>
      ) : (
        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="text-xs font-medium text-muted-foreground">Code</label>
            <div className="flex items-center gap-2">
              {syntaxResult && syntaxResult.ok && (
                <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
                  <CheckCircle className="h-3.5 w-3.5" />
                  Syntax OK
                </span>
              )}
              {syntaxResult && !syntaxResult.ok && (
                <span className="flex items-center gap-1 text-xs text-destructive truncate max-w-xs">
                  <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                  {syntaxResult.line !== undefined
                    ? `Line ${syntaxResult.line}, col ${syntaxResult.col ?? 0}: ${syntaxResult.error}`
                    : syntaxResult.error}
                </span>
              )}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleCheckSyntax}
                disabled={validateMutation.isPending}
              >
                {validateMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" />}
                Check syntax
              </Button>
            </div>
          </div>
          <CodeEditor
            value={code}
            onChange={setCode}
            language="python"
            placeholder="# Python script..."
            minHeight="240px"
            maxHeight="500px"
          />
        </div>
      )}
      <div className="flex gap-2 justify-end">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={isPending}>
          {isCreate ? 'Create' : 'Save'}
        </Button>
      </div>
      <FilePickerModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onSelect={(path) => {
          setFilePath(path)
          setModalOpen(false)
        }}
      />
    </form>
  )
}
