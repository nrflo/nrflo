import { useState, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'

interface TooltipProps {
  text: string
  children: ReactNode
  placement?: 'top' | 'bottom' | 'left' | 'right'
  className?: string
}

export function Tooltip({ text, children, placement = 'top', className }: TooltipProps) {
  const [visible, setVisible] = useState(false)
  const [coords, setCoords] = useState({ top: 0, left: 0 })
  const triggerRef = useRef<HTMLDivElement>(null)

  const show = () => {
    if (!triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    const gap = 6

    let top = 0
    let left = 0

    switch (placement) {
      case 'top':
        top = rect.top - gap
        left = rect.left + rect.width / 2
        break
      case 'bottom':
        top = rect.bottom + gap
        left = rect.left + rect.width / 2
        break
      case 'left':
        top = rect.top + rect.height / 2
        left = rect.left - gap
        break
      case 'right':
        top = rect.top + rect.height / 2
        left = rect.right + gap
        break
    }

    setCoords({ top, left })
    setVisible(true)
  }

  const placementStyles: Record<string, string> = {
    top: '-translate-x-1/2 -translate-y-full',
    bottom: '-translate-x-1/2',
    left: '-translate-x-full -translate-y-1/2',
    right: '-translate-y-1/2',
  }

  return (
    <>
      <div
        ref={triggerRef}
        onMouseEnter={show}
        onMouseLeave={() => setVisible(false)}
        className="inline-flex"
      >
        {children}
      </div>
      {visible && createPortal(
        <div
          className={cn(
            'fixed z-[100] px-2 py-1 text-xs rounded bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 shadow-lg pointer-events-none whitespace-nowrap',
            placementStyles[placement],
            className
          )}
          style={{ top: coords.top, left: coords.left }}
        >
          {text}
        </div>,
        document.body
      )}
    </>
  )
}
