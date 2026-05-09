import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'

interface HamburgerMenuProps {
  children: ReactNode
  /** Extra modifier classes for the root element. Lets consumers
   *  encode visual states like login status via CSS modifiers
   *  (e.g. 'hamburger--authed') without coupling this component to
   *  auth concerns. */
  className?: string
}

// Wraps content that lives inline on desktop and collapses behind a
// hamburger button on mobile. The wrapper uses `display: contents` on
// desktop so its children participate directly in the parent's flex
// layout (no visual change). On mobile the wrapper becomes a sticky
// right-edge button + popup.
export function HamburgerMenu({ children, className }: HamburgerMenuProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const location = useLocation()

  // Close on outside click.
  useEffect(() => {
    if (!open) return
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [open])

  // Close on route change (so a nav-link tap inside the popup actually
  // dismisses the menu after navigating).
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setOpen(false) }, [location.pathname])

  // Close on Esc.
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open])

  return (
    <div className={`hamburger ${open ? 'hamburger--open' : ''} ${className ?? ''}`} ref={ref}>
      <button
        type="button"
        className="hamburger__btn"
        onClick={() => setOpen((v) => !v)}
        aria-label={open ? 'Close menu' : 'Open menu'}
        aria-expanded={open}
        aria-haspopup="menu"
      >
        <svg width="18" height="14" viewBox="0 0 18 14" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
          <line x1="2" y1="3" x2="16" y2="3" />
          <line x1="2" y1="7" x2="16" y2="7" />
          <line x1="2" y1="11" x2="16" y2="11" />
        </svg>
      </button>
      <div className="hamburger__popup" role="menu">
        {children}
      </div>
    </div>
  )
}
