import { type ReactNode } from 'react'
import * as RadixTooltip from '@radix-ui/react-tooltip'
import { cn } from '@/lib/utils'

interface TooltipProps {
  text: ReactNode
  children: ReactNode
  placement?: 'top' | 'bottom' | 'left' | 'right'
  className?: string
}

export function Tooltip({ text, children, placement = 'top', className }: TooltipProps) {
  return (
    <RadixTooltip.Provider delayDuration={200}>
      <RadixTooltip.Root>
        <RadixTooltip.Trigger asChild>
          <span className="inline-flex">
            {children}
          </span>
        </RadixTooltip.Trigger>
        <RadixTooltip.Portal>
          <RadixTooltip.Content
            side={placement}
            sideOffset={6}
            className={cn(
              'z-[100] px-2 py-1 text-xs rounded bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 shadow-lg',
              'animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95',
              className
            )}
          >
            {text}
          </RadixTooltip.Content>
        </RadixTooltip.Portal>
      </RadixTooltip.Root>
    </RadixTooltip.Provider>
  )
}
