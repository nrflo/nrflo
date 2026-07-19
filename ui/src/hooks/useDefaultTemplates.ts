import { useQuery } from '@tanstack/react-query'
import { listDefaultTemplates } from '@/api/defaultTemplates'

export const templateKeys = {
  all: ['default-templates'] as const,
  list: (type?: string) => [...templateKeys.all, 'list', type ?? null] as const,
}

export function useInjectableTemplates() {
  return useQuery({
    queryKey: templateKeys.list('injectable'),
    queryFn: () => listDefaultTemplates('injectable'),
  })
}
