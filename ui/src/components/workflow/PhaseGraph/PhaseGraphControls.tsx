import { useEffect, useRef } from 'react'
import { Controls, ControlButton, useReactFlow } from '@xyflow/react'
import { Minus, Plus, Maximize2 } from 'lucide-react'
import { Tooltip } from '@/components/ui/Tooltip'
import { performFitView } from './fitViewOptions'

const AUTO_CENTER_INTERVAL_MS = 15000

/**
 * When enabled, calls fitView(FIT_VIEW_OPTIONS) every 15 seconds.
 * Keeps fitView in a ref so WS-driven parent re-renders do not tear down and
 * re-arm the interval before it fires (regression on the ticket workflow page).
 */
export function AutoCenterInterval({ enabled }: { enabled: boolean }) {
  const { fitView } = useReactFlow()
  const fitViewRef = useRef(fitView)

  useEffect(() => {
    fitViewRef.current = fitView
  }, [fitView])

  useEffect(() => {
    if (!enabled) return
    const id = setInterval(() => performFitView(fitViewRef.current), AUTO_CENTER_INTERVAL_MS)
    return () => clearInterval(id)
  }, [enabled])

  return null
}

interface PhaseGraphControlsProps {
  autoCenter: boolean
  setAutoCenter: (value: boolean) => void
}

export function PhaseGraphControls({ autoCenter, setAutoCenter }: PhaseGraphControlsProps) {
  const { zoomIn, zoomOut, fitView } = useReactFlow()

  const handleZoomOut = () => {
    setAutoCenter(false)
    zoomOut()
  }

  const handleZoomIn = () => {
    setAutoCenter(false)
    zoomIn()
  }

  const handleFitView = () => {
    setAutoCenter(false)
    performFitView(fitView)
  }

  return (
    <Controls showInteractive={false} position="top-left">
      <ControlButton onClick={handleZoomOut} aria-label="zoom out" title="Zoom out">
        <Minus size={14} />
      </ControlButton>
      <ControlButton onClick={handleZoomIn} aria-label="zoom in" title="Zoom in">
        <Plus size={14} />
      </ControlButton>
      <ControlButton onClick={handleFitView} aria-label="fit view" title="Fit view">
        <Maximize2 size={14} />
      </ControlButton>
      <ControlButton
        aria-label="auto center graph"
        onClick={(e) => e.stopPropagation()}
      >
        <Tooltip text="Auto center graph every 15s">
          <input
            type="checkbox"
            aria-label="Auto center graph every 15s"
            checked={autoCenter}
            onChange={(e) => setAutoCenter(e.target.checked)}
            onClick={(e) => e.stopPropagation()}
            style={{ width: 12, height: 12, cursor: 'pointer', margin: 0 }}
          />
        </Tooltip>
      </ControlButton>
    </Controls>
  )
}
