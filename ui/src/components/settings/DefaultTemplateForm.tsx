import { X, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'

export interface TemplateFormData {
  id: string
  name: string
  type: string
  template: string
}

export const emptyTemplateForm: TemplateFormData = {
  id: '',
  name: '',
  type: 'agent',
  template: '',
}

const typeOptions = [
  { value: 'agent', label: 'Agent' },
  { value: 'injectable', label: 'Injectable' },
]

export function DefaultTemplateForm({
  formData,
  setFormData,
  onCancel,
  onSave,
  mutation,
  isCreate,
  isReadonly,
  isModified,
  onRestore,
  isRestoring,
}: {
  formData: TemplateFormData
  setFormData: (data: TemplateFormData) => void
  onCancel: () => void
  onSave: () => void
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutation: { isPending: boolean; isError: boolean; error: any }
  isCreate?: boolean
  isReadonly?: boolean
  isModified?: boolean
  onRestore?: () => void
  isRestoring?: boolean
}) {
  return (
    <div className={`space-y-3 ${isCreate ? 'border border-primary rounded-lg p-4 bg-muted/30' : ''}`}>
      <div className="grid grid-cols-3 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            ID {isCreate && <span className="text-destructive">*</span>}
          </label>
          {isCreate ? (
            <Input
              value={formData.id}
              onChange={(e) => setFormData({ ...formData, id: e.target.value })}
              placeholder="my-template"
            />
          ) : (
            <Input value={formData.id} disabled className="bg-muted" />
          )}
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            Name {isCreate && <span className="text-destructive">*</span>}
          </label>
          <Input
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            placeholder="Template name"
            disabled={isReadonly}
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Type</label>
          {isCreate ? (
            <Dropdown
              value={formData.type}
              onChange={(val) => setFormData({ ...formData, type: val })}
              options={typeOptions}
            />
          ) : (
            <Input value={formData.type} disabled className="bg-muted" />
          )}
        </div>
      </div>
      <div>
        <label className="text-sm font-medium text-muted-foreground">
          Template {isCreate && <span className="text-destructive">*</span>}
        </label>
        <MarkdownEditor
          value={formData.template}
          onChange={(val) => setFormData({ ...formData, template: val })}
          placeholder="Agent prompt template..."
          readOnly={false}
          minHeight="200px"
          maxHeight="400px"
        />
      </div>
      {formData.type === 'injectable' && (
        <div className="bg-muted/50 border rounded p-3 text-xs space-y-1.5">
          <div className="font-medium text-muted-foreground">Injectable Placeholders</div>
          <div className="space-y-1 font-mono">
            <div><span className="text-primary">{'${USER_INSTRUCTIONS}'}</span> — User-provided instructions</div>
            <div><span className="text-primary">{'${PREVIOUS_DATA}'}</span> — Saved state from previous run</div>
            <div><span className="text-primary">{'${CALLBACK_INSTRUCTIONS}'}</span> — Callback instructions</div>
            <div><span className="text-primary">{'${CALLBACK_FROM_AGENT}'}</span> — Agent that triggered callback</div>
          </div>
        </div>
      )}
      <div className="flex gap-2 justify-end">
        <Button variant="ghost" onClick={onCancel}>
          {isCreate ? 'Cancel' : <><X className="h-4 w-4 mr-1" />Cancel</>}
        </Button>
        {isModified && onRestore && (
          <Button variant="outline" onClick={onRestore} disabled={isRestoring}>
            {isRestoring ? 'Restoring...' : 'Restore Default'}
          </Button>
        )}
        <Button
          onClick={onSave}
          disabled={
            isCreate
              ? !formData.id.trim() || !formData.name.trim() || !formData.template.trim() || mutation.isPending
              : mutation.isPending
          }
        >
          {isCreate ? (
            mutation.isPending ? 'Creating...' : 'Create'
          ) : (
            <>{mutation.isPending ? 'Saving...' : <><Check className="h-4 w-4 mr-1" />Save</>}</>
          )}
        </Button>
      </div>
      {mutation.isError && (
        <p className="text-sm text-destructive">
          Error: {mutation.error.message}
        </p>
      )}
    </div>
  )
}
