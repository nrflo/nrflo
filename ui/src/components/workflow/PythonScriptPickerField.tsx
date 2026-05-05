import { Link } from 'react-router-dom'
import { Dropdown } from '@/components/ui/Dropdown'
import { usePythonScripts } from '@/hooks/usePythonScripts'

interface PythonScriptPickerFieldProps {
  value: string
  onChange: (id: string) => void
}

export function PythonScriptPickerField({ value, onChange }: PythonScriptPickerFieldProps) {
  const { data: scripts = [], isLoading } = usePythonScripts()

  if (isLoading) {
    return <div className="text-xs text-muted-foreground py-2">Loading scripts…</div>
  }

  if (scripts.length === 0) {
    return (
      <p className="text-xs text-muted-foreground py-2">
        No scripts yet —{' '}
        <Link to="/python-scripts" className="underline text-primary">
          create one on the Python Scripts page
        </Link>
        .
      </p>
    )
  }

  const options = scripts.map((s) => ({
    value: s.id,
    label: s.description
      ? `${s.name} — ${s.description.slice(0, 60)}${s.description.length > 60 ? '…' : ''}`
      : s.name,
  }))

  return (
    <Dropdown
      value={value}
      onChange={onChange}
      options={options}
      placeholder="Select a Python script…"
    />
  )
}
