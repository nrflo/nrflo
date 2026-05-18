import { useState } from 'react'
import { AlertCircle, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { useReadPythonFile } from '@/hooks/usePythonScripts'
import { FilePickerModal } from './FilePickerModal'
import type { PythonScript, PythonToolCreateRequest, PythonToolUpdateRequest, ValidationResult } from '@/types/pythonScript'

export type ToolFormData = PythonToolCreateRequest | PythonToolUpdateRequest

interface PythonToolFormProps {
  initial?: PythonScript
  isCreate: boolean
  onSubmit: (data: ToolFormData) => void
  onValidationFailure: (result: ValidationResult, data: ToolFormData) => void
  onCancel: () => void
  isPending?: boolean
}

export function PythonToolForm({
  initial,
  isCreate,
  onSubmit,
  onValidationFailure,
  onCancel,
  isPending,
}: PythonToolFormProps) {
  const [name, setName] = useState(initial?.name ?? '')
  const [description, setDescription] = useState(initial?.description ?? '')
  const [toolDescription, setToolDescription] = useState(initial?.tool_description ?? '')
  const [inputSchema, setInputSchema] = useState(
    initial?.input_schema ? (typeof initial.input_schema === 'string' ? initial.input_schema : JSON.stringify(initial.input_schema, null, 2)) : ''
  )
  const [schemaError, setSchemaError] = useState<string | null>(null)
  const [timeoutSec, setTimeoutSec] = useState<number>(initial?.timeout_sec ?? 30)
  const [timeoutError, setTimeoutError] = useState<string | null>(null)
  const [code, setCode] = useState(initial?.code ?? '')
  const [filePath, setFilePath] = useState(initial?.file_path ?? '')
  const [modalOpen, setModalOpen] = useState(false)

  const fileQuery = useReadPythonFile(filePath || null)

  const validateLocalFields = (): boolean => {
    let valid = true

    if (!name.trim() || !toolDescription.trim()) {
      valid = false
    }

    try {
      JSON.parse(inputSchema)
      setSchemaError(null)
    } catch {
      setSchemaError('Invalid JSON')
      valid = false
    }

    if (timeoutSec < 1 || timeoutSec > 600 || !Number.isInteger(timeoutSec)) {
      setTimeoutError('Must be 1–600')
      valid = false
    } else {
      setTimeoutError(null)
    }

    return valid
  }

  const buildFormData = (): ToolFormData => {
    if (isCreate) {
      return {
        kind: 'tool',
        name,
        description: description || undefined,
        tool_description: toolDescription,
        input_schema: inputSchema,
        timeout_sec: timeoutSec,
        code,
        file_path: filePath || undefined,
      } as PythonToolCreateRequest
    }
    return {
      name,
      description: description || undefined,
      tool_description: toolDescription,
      input_schema: inputSchema,
      timeout_sec: timeoutSec,
      code,
      file_path: filePath || undefined,
    } as PythonToolUpdateRequest
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!validateLocalFields()) return

    const data = buildFormData()

    if (filePath) {
      onSubmit(data)
      return
    }

    // Validate code is non-empty when using inline template
    if (!code.trim()) {
      const result: ValidationResult = { ok: false, error: 'Code is required' }
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
          placeholder="e.g., search-web"
          required
        />
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Description</label>
        <Textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional internal description"
          rows={2}
        />
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Tool description <span className="text-destructive">*</span>
        </label>
        <Textarea
          value={toolDescription}
          onChange={(e) => setToolDescription(e.target.value)}
          placeholder="Describe what this tool does for the LLM"
          rows={3}
          required
        />
      </div>
      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="text-xs font-medium text-muted-foreground">
            Input schema (JSON) <span className="text-destructive">*</span>
          </label>
          {schemaError && (
            <span className="flex items-center gap-1 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 shrink-0" />
              {schemaError}
            </span>
          )}
        </div>
        <CodeEditor
          value={inputSchema}
          onChange={(v) => { setInputSchema(v); setSchemaError(null) }}
          language="plain"
          placeholder='{"type":"object","properties":{}}'
          minHeight="100px"
          maxHeight="200px"
        />
      </div>
      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="text-xs font-medium text-muted-foreground">Timeout (seconds)</label>
          {timeoutError && (
            <span className="flex items-center gap-1 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 shrink-0" />
              {timeoutError}
            </span>
          )}
        </div>
        <Input
          type="number"
          min={1}
          max={600}
          value={timeoutSec}
          onChange={(e) => { setTimeoutSec(Number(e.target.value)); setTimeoutError(null) }}
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
        <Input value={filePath} disabled placeholder="(none — using template below)" />
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
            value={fileQuery.data?.content ?? ''}
            onChange={() => {}}
            language="python"
            readOnly
            minHeight="240px"
            maxHeight="500px"
          />
        </div>
      ) : (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Code</label>
          <CodeEditor
            value={code}
            onChange={setCode}
            language="python"
            placeholder="# Python tool implementation..."
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
