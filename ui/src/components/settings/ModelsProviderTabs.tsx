import { useState } from 'react'
import { Plus } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import { useCustomProviders, useCreateCustomProvider } from '@/hooks/useCustomProviders'
import { CustomProviderForm, emptyCustomProviderForm, type CustomProviderFormData } from './CustomProviderForm'

export const BUILTIN_PROVIDERS: { id: string; label: string }[] = [
  { id: 'anthropic', label: 'Anthropic' },
  { id: 'openai', label: 'OpenAI' },
  { id: 'openrouter', label: 'OpenRouter' },
]

export const BUILTIN_PROVIDER_IDS = new Set<string>(BUILTIN_PROVIDERS.map((p) => p.id))

interface Props {
  activeProvider: string
  onSelect: (provider: string) => void
}

export function ModelsProviderTabs({ activeProvider, onSelect }: Props) {
  const { data: customProviders = [] } = useCustomProviders()
  const createMutation = useCreateCustomProvider()
  const [adding, setAdding] = useState(false)
  const [formData, setFormData] = useState<CustomProviderFormData>(emptyCustomProviderForm)

  const tabs = [
    ...BUILTIN_PROVIDERS,
    ...customProviders.map((p) => ({ id: p.name, label: p.name })),
  ]

  const startAdd = () => {
    setFormData(emptyCustomProviderForm)
    setAdding(true)
  }
  const cancelAdd = () => setAdding(false)
  const saveAdd = () => createMutation.mutate(
    { name: formData.name.trim(), base_url: formData.base_url.trim(), api_key: formData.api_key, api_wire: formData.api_wire },
    { onSuccess: (created) => { setAdding(false); onSelect(created.name) } },
  )

  return (
    <div className="space-y-3">
      <div className="border-b border-border">
        <div className="flex flex-wrap items-center gap-1">
          {tabs.map(({ id, label }) => (
            <button
              key={id}
              onClick={() => onSelect(id)}
              className={cn(
                'flex items-center gap-2 px-3 py-1 text-xs font-medium border-b-2 transition-colors',
                activeProvider === id
                  ? 'border-primary text-primary'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              )}
            >
              {label}
            </button>
          ))}
          <Button variant="ghost" size="sm" onClick={startAdd} disabled={adding} className="ml-1">
            <Plus className="mr-1 h-3.5 w-3.5" />Add provider
          </Button>
        </div>
      </div>
      {adding && (
        <CustomProviderForm formData={formData} setFormData={setFormData} onCancel={cancelAdd} onSave={saveAdd} mutation={createMutation} isCreate />
      )}
    </div>
  )
}
