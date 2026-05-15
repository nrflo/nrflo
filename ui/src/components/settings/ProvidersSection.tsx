import { ProviderModelsList } from './ProviderModelsList'
import type { ProviderName } from '@/api/providers'


interface Props {
  activeProvider: ProviderName
}

export function ProvidersSection({ activeProvider }: Props) {
  return (
    <div className="space-y-4">
      <ProviderModelsList provider={activeProvider} />
    </div>
  )
}
