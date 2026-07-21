import { useMemo, useState } from 'react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Dropdown } from '@/components/ui/Dropdown'
import { Toggle } from '@/components/ui/Toggle'
import { Button } from '@/components/ui/Button'
import { useInvokeChatTool } from '@/hooks/useChatTools'
import { TurnActiveError } from '@/api/consoleChats'
import { parseSchema, initialValues, buildArguments, type FieldDescriptor } from './chatInvokeSchema'
import type { ConsoleTool } from '@/types/consoleChat'

interface ChatInvokeFormProps {
  sid: string
  tool: ConsoleTool
  onClose: () => void
}

// Schema-driven argument form for the '/invoke' directive: one control per
// top-level input_schema property, an "Inform model" toggle, and Run/Cancel.
// Run POSTs /console/chats/{sid}/invoke; the resulting transcript rows
// arrive over the existing messages.updated WS event, so success just
// closes the form — no local echo/rendering here.
export function ChatInvokeForm({ sid, tool, onClose }: ChatInvokeFormProps) {
  const fields = useMemo(() => parseSchema(tool.input_schema), [tool.input_schema])
  const [values, setValues] = useState(() => initialValues(fields))
  const [informModel, setInformModel] = useState(true)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState<string | null>(null)
  const invoke = useInvokeChatTool()

  const setValue = (name: string, v: string | boolean) => {
    setValues((prev) => ({ ...prev, [name]: v }))
  }

  const handleRun = async () => {
    setFormError(null)
    const { args, errors } = buildArguments(fields, values)
    setFieldErrors(errors)
    if (Object.keys(errors).length > 0) return

    try {
      await invoke.mutateAsync({ sid, tool: tool.name, arguments: args, inform_model: informModel })
      onClose()
    } catch (e) {
      if (e instanceof TurnActiveError) {
        setFormError('A turn is already running.')
      } else {
        setFormError(e instanceof Error ? e.message : 'Failed to invoke tool.')
      }
    }
  }

  return (
    <Dialog open onClose={onClose}>
      <DialogHeader onClose={onClose}>
        <div className="font-mono text-sm">{tool.name}</div>
        {tool.description && <div className="text-xs text-muted-foreground mt-0.5">{tool.description}</div>}
      </DialogHeader>
      <DialogBody className="space-y-3">
        {fields.length === 0 && <div className="text-sm text-muted-foreground">This tool takes no arguments.</div>}
        {fields.map((field) => (
          <Field
            key={field.name}
            field={field}
            value={values[field.name]}
            error={fieldErrors[field.name]}
            onChange={(v) => setValue(field.name, v)}
          />
        ))}
        {formError && <div className="text-sm text-destructive">{formError}</div>}
      </DialogBody>
      <DialogFooter className="justify-between">
        <Toggle checked={informModel} onChange={setInformModel} label="Inform model" />
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleRun} disabled={invoke.isPending}>
            Run
          </Button>
        </div>
      </DialogFooter>
    </Dialog>
  )
}

interface FieldProps {
  field: FieldDescriptor
  value: string | boolean
  error?: string
  onChange: (v: string | boolean) => void
}

function Field({ field, value, error, onChange }: FieldProps) {
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium text-foreground">
        {field.name}
        {field.required && <span className="text-destructive"> *</span>}
      </label>
      {field.description && <div className="text-xs text-muted-foreground">{field.description}</div>}
      <FieldControl field={field} value={value} onChange={onChange} />
      {error && <div className="text-xs text-destructive">{error}</div>}
    </div>
  )
}

function FieldControl({ field, value, onChange }: FieldProps) {
  if (field.kind === 'boolean') {
    return <Toggle checked={Boolean(value)} onChange={onChange} />
  }
  if (field.kind === 'enum') {
    return (
      <Dropdown
        value={String(value)}
        onChange={onChange}
        options={(field.enumOptions ?? []).map((o) => ({ value: o, label: o }))}
        placeholder="Select…"
      />
    )
  }
  if (field.kind === 'number') {
    return <Input type="number" value={String(value)} onChange={(e) => onChange(e.target.value)} />
  }
  if (field.kind === 'json') {
    return (
      <Textarea
        value={String(value)}
        onChange={(e) => onChange(e.target.value)}
        rows={3}
        className="font-mono text-xs"
      />
    )
  }
  return <Input value={String(value)} onChange={(e) => onChange(e.target.value)} />
}
