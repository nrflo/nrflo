import { X, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'

export interface TemplateFormData {
  id: string
  name: string
  template: string
}

export const emptyTemplateForm: TemplateFormData = {
  id: '',
  name: '',
  template: '',
}

export function DefaultTemplateForm({
  formData,
  setFormData,
  onCancel,
  onSave,
  mutation,
  isCreate,
  isReadonly,
}: {
  formData: TemplateFormData
  setFormData: (data: TemplateFormData) => void
  onCancel: () => void
  onSave: () => void
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutation: { isPending: boolean; isError: boolean; error: any }
  isCreate?: boolean
  isReadonly?: boolean
}) {
  return (
    <div className={`space-y-3 ${isCreate ? 'border border-primary rounded-lg p-4 bg-muted/30' : ''}`}>
      {isReadonly && (
        <p className="text-sm text-muted-foreground italic">
          This is a built-in template and cannot be modified.
        </p>
      )}
      <div className="grid grid-cols-2 gap-3">
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
      </div>
      <div>
        <label className="text-sm font-medium text-muted-foreground">
          Template {isCreate && <span className="text-destructive">*</span>}
        </label>
        <MarkdownEditor
          value={formData.template}
          onChange={(val) => setFormData({ ...formData, template: val })}
          placeholder="Agent prompt template..."
          readOnly={isReadonly}
          minHeight="200px"
          maxHeight="400px"
        />
      </div>
      <div className="flex gap-2 justify-end">
        <Button variant="ghost" onClick={onCancel}>
          {isCreate ? 'Cancel' : <><X className="h-4 w-4 mr-1" />Cancel</>}
        </Button>
        {!isReadonly && (
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
        )}
      </div>
      {mutation.isError && (
        <p className="text-sm text-destructive">
          Error: {mutation.error.message}
        </p>
      )}
    </div>
  )
}
