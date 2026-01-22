import { useState, useRef } from 'react'
import { createPortal } from 'react-dom'

interface PlayerPortraitProps {
  model?: string  // e.g., "sarge/default", "sarge", or "*james"
  size?: 'sm' | 'md' | 'lg' | 'xl'
  fallback?: React.ReactNode
  className?: string
}

const SIZE_CLASSES: Record<NonNullable<PlayerPortraitProps['size']>, string> = {
  sm: 'portrait-sm',
  md: 'portrait-md',
  lg: 'portrait-lg',
  xl: 'portrait-xl',
}

/**
 * Parse a Q3A model string to get the portrait path.
 * Examples:
 * - "sarge" -> /assets/portraits/sarge/icon_default.png
 * - "sarge/krusade" -> /assets/portraits/sarge/icon_krusade.png
 * - "*james" -> /assets/portraits/james/icon_default.png (Team Arena head)
 * - "*Callisto/blue" -> /assets/portraits/callisto/icon_blue.png
 */
function getPortraitPath(model: string): string {
  // Strip Team Arena asterisk prefix
  let cleanModel = model.startsWith('*') ? model.slice(1) : model

  // Split into model name and skin
  const parts = cleanModel.split('/')
  const modelName = parts[0].toLowerCase()
  const skin = (parts[1] || 'default').toLowerCase()

  return `/assets/portraits/${modelName}/icon_${skin}.png`
}

const DEFAULT_PORTRAIT = '/assets/portraits/sarge/icon_default.png'

export function PlayerPortrait({ model, size = 'sm', fallback, className = '' }: PlayerPortraitProps) {
  const [hasError, setHasError] = useState(false)
  const [showPreview, setShowPreview] = useState(false)
  const [previewPos, setPreviewPos] = useState({ x: 0, y: 0 })
  const ref = useRef<HTMLSpanElement>(null)
  const sizeClass = SIZE_CLASSES[size]

  // No model provided
  if (!model) {
    if (fallback) {
      return <span className={`player-portrait ${sizeClass} ${className}`}>{fallback}</span>
    }
    return null
  }

  const src = hasError ? DEFAULT_PORTRAIT : getPortraitPath(model)
  const showHoverPreview = size === 'sm' || size === 'md'

  const handleMouseEnter = () => {
    if (!showHoverPreview || !ref.current) return
    const rect = ref.current.getBoundingClientRect()
    setPreviewPos({
      x: rect.left + rect.width / 2,
      y: rect.top,
    })
    setShowPreview(true)
  }

  const handleMouseLeave = () => {
    setShowPreview(false)
  }

  return (
    <>
      <span
        ref={ref}
        className={`player-portrait ${sizeClass} ${className}`}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
      >
        <img
          src={src}
          alt={model}
          onError={hasError ? undefined : () => setHasError(true)}
        />
      </span>
      {showPreview && createPortal(
        <div
          className="portrait-preview"
          style={{
            left: previewPos.x,
            top: previewPos.y,
          }}
        >
          <img src={src} alt={model} />
        </div>,
        document.body
      )}
    </>
  )
}
