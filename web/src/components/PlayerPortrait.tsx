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

const PORTRAIT_HELP = `Player model — what they look like in-game.`

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
  const cleanModel = model.startsWith('*') ? model.slice(1) : model

  // Split into model name and skin
  const parts = cleanModel.split('/')
  const modelName = parts[0].toLowerCase()
  const skin = (parts[1] || 'default').toLowerCase()

  return `/assets/portraits/${modelName}/icon_${skin}.png`
}

// Head-and-shoulders silhouette shown when no model is provided or the
// portrait PNG fails to load. Inline SVG (rather than a data URI) so it
// can pick up theme tokens via var() and react to dark-mode changes.
function DefaultPortraitSvg() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 128 128"
      preserveAspectRatio="xMidYMid meet"
      aria-hidden="true"
    >
      <rect width="128" height="128" rx="4" fill="var(--bg-elev)" />
      <circle cx="64" cy="46" r="28" fill="var(--text-dim)" />
      <path
        d="M64,78 C39,78 16,96 16,110 L16,128 L112,128 L112,110 C112,96 89,78 64,78Z"
        fill="var(--text-dim)"
      />
    </svg>
  )
}

export function PlayerPortrait({ model, size = 'sm', fallback, className = '' }: PlayerPortraitProps) {
  const [hasError, setHasError] = useState(false)
  const [showPreview, setShowPreview] = useState(false)
  const [previewPos, setPreviewPos] = useState({ x: 0, y: 0 })
  const ref = useRef<HTMLSpanElement>(null)
  const sizeClass = SIZE_CLASSES[size]

  // No model provided — fall back to the caller's fallback (a single
  // initial letter, in practice) rather than the generic silhouette.
  // Silhouette is only used as a last resort when neither model nor
  // fallback is available — which shouldn't happen in normal flows.
  if (!model) {
    return (
      <span className={`player-portrait ${sizeClass} ${className}`} data-help={PORTRAIT_HELP}>
        {fallback ?? <DefaultPortraitSvg />}
      </span>
    )
  }

  const showHoverPreview = size === 'sm' || size === 'md'

  const handleMouseEnter = () => {
    if (!showHoverPreview || !ref.current) return
    // Inside a HelpModeRoot the help-popover *is* the preview-and-
    // explanation surface; suppress the magnified-portrait popup so
    // the two don't fight for the same screen space.
    if (ref.current.closest('.help-mode-root')) return
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

  const portraitSrc = getPortraitPath(model)

  return (
    <>
      <span
        ref={ref}
        className={`player-portrait ${sizeClass} ${className}`}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
        data-help={PORTRAIT_HELP}
      >
        {hasError ? (
          // Prefer the caller's fallback (the player's initial) over
          // the silhouette when the model's portrait fails to load.
          // Silhouette only renders if no fallback was supplied — a
          // case our card components don't produce in practice.
          fallback ?? <DefaultPortraitSvg />
        ) : (
          <img
            src={portraitSrc}
            alt={model}
            onError={() => setHasError(true)}
          />
        )}
      </span>
      {showPreview && createPortal(
        <div
          className="portrait-preview"
          style={{
            left: previewPos.x,
            top: previewPos.y,
          }}
        >
          {hasError ? (
            <DefaultPortraitSvg />
          ) : (
            <img src={portraitSrc} alt={model} />
          )}
        </div>,
        document.body
      )}
    </>
  )
}
