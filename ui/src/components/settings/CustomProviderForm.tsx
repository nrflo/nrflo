import { Check, X } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Input } from '@/components/ui/Input'
import { Toggle } from '@/components/ui/Toggle'
import type { CustomProvider, APIWire } from '@/api/customProviders'
import { ProviderConnectionCheckButton } from './ProviderConnectionCheckButton'

const API_WIRE_OPTIONS: { value: APIWire; label: string }[] = [
  { value: 'responses', label: 'Responses API' },
  { value: 'chat_completions', label: 'Chat Completions API' },
]

export interface CustomProviderFormData {
  name: string
  base_url: string
  api_key: string
  api_wire: APIWire
  enabled: boolean
}

export const emptyCustomProviderForm: CustomProviderFormData = {
  name: '', base_url: '', api_key: '', api_wire: 'responses', enabled: true,
}

// Edit mode starts api_key blank (write-mostly): a blank field means "keep
// the current key" and is omitted from the PATCH request by the caller.
export function providerToFormData(provider: CustomProvider): CustomProviderFormData {
  return {
    name: provider.name, base_url: provider.base_url, api_key: '',
    api_wire: provider.api_wire as APIWire, enabled: provider.enabled,
  }
}

interface Props {
  formData: CustomProviderFormData
  setFormData: (data: CustomProviderFormData) => void
  onCancel: () => void
  onSave: () => void
  mutation: { isPending: boolean; isError: boolean; error: Error | null }
  isCreate?: boolean
  /** Current stored key, used to test the connection when the field is left blank in edit mode. */
  existingApiKey?: string
}

export function CustomProviderForm({ formData, setFormData, onCancel, onSave, mutation, isCreate, existingApiKey }: Props) {
  const update = (patch: Partial<CustomProviderFormData>) => setFormData({ ...formData, ...patch })
  const valid = formData.name.trim() && formData.base_url.trim()

  return (
    <div className={`space-y-4 ${isCreate ? 'rounded-lg border border-primary bg-muted/30 p-4' : ''}`}>
      <div className="grid grid-cols-3 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">Name</label>
          <Input value={formData.name} disabled={!isCreate} onChange={(e) => update({ name: e.target.value })} placeholder="my-provider" />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Base URL</label>
          <Input value={formData.base_url} onChange={(e) => update({ base_url: e.target.value })} placeholder="https://api.example.com/v1" />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">API Wire</label>
          <Dropdown value={formData.api_wire} onChange={(value) => update({ api_wire: value as APIWire })} options={API_WIRE_OPTIONS} />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3 items-end">
        <div>
          <label className="text-sm font-medium text-muted-foreground">API Key</label>
          <Input
            type="password"
            value={formData.api_key}
            placeholder={isCreate ? '' : 'Leave blank to keep current key'}
            onChange={(e) => update({ api_key: e.target.value })}
          />
        </div>
        <div className="flex items-center gap-3">
          <ProviderConnectionCheckButton baseUrl={formData.base_url} apiKey={formData.api_key || existingApiKey || ''} apiWire={formData.api_wire} />
          <Toggle checked={formData.enabled} onChange={(checked) => update({ enabled: checked })} label="Enabled" />
        </div>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel}>{isCreate ? 'Cancel' : <><X className="mr-1 h-4 w-4" />Cancel</>}</Button>
        <Button onClick={onSave} disabled={!valid || mutation.isPending}>
          {mutation.isPending ? 'Saving...' : isCreate ? 'Create' : <><Check className="mr-1 h-4 w-4" />Save</>}
        </Button>
      </div>
      {mutation.isError && <p className="text-sm text-destructive">Error: {mutation.error?.message}</p>}
    </div>
  )
}
