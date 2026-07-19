import { Dropdown } from '@/components/ui/Dropdown'
import { useInjectableTemplates } from '@/hooks/useDefaultTemplates'

export function AgentDefSystemTemplateField({
  value,
  onChange,
}: {
  value: string
  onChange: (v: string) => void
}) {
  const { data: templates = [] } = useInjectableTemplates()

  const options = [
    { value: '', label: 'Default (global rules)' },
    ...templates.map((t) => ({ value: t.id, label: t.name })),
  ]

  return (
    <div className="flex-1">
      <label className="block text-xs font-medium text-muted-foreground mb-1">System template</label>
      <Dropdown value={value} onChange={onChange} options={options} />
    </div>
  )
}
