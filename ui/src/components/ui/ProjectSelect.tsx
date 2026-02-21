import { FolderOpen } from 'lucide-react'
import { Dropdown } from './Dropdown'

interface ProjectOption {
  id: string
  name: string
}

interface ProjectSelectProps {
  value: string
  onChange: (value: string) => void
  projects: ProjectOption[]
}

export function ProjectSelect({ value, onChange, projects }: ProjectSelectProps) {
  const options = projects.map((p) => ({ value: p.id, label: p.name }))

  return (
    <Dropdown
      value={value}
      onChange={onChange}
      options={options}
      icon={<FolderOpen className="h-4 w-4 text-muted-foreground" />}
      className="w-auto"
      labelClassName="hidden md:inline"
    />
  )
}
