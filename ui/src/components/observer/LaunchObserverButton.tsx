import { Eye } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { useExperimentalObserverEnabled } from '@/hooks/useGlobalSettings'
import { useLaunchObserver } from '@/hooks/useObservers'
import type { ObserverLaunchRequest } from '@/api/observers'

interface LaunchObserverButtonProps {
  payload: ObserverLaunchRequest
  size?: 'sm' | 'default' | 'lg'
  variant?: 'outline' | 'default' | 'ghost'
}

export function LaunchObserverButton({ payload, size = 'sm', variant = 'outline' }: LaunchObserverButtonProps) {
  const enabled = useExperimentalObserverEnabled()
  const mutation = useLaunchObserver()

  if (!enabled) return null

  return (
    <Button
      variant={variant}
      size={size}
      onClick={() => mutation.mutate(payload)}
      disabled={mutation.isPending}
      title="Launch observer"
    >
      <Eye className="h-4 w-4 mr-2" />
      Observer
    </Button>
  )
}
