import { useEffect } from 'react'

export interface HotkeySpec {
  // Lowercase key name (e.g. 'k', '/', 'escape').
  key: string
  // True if the modifier (Cmd on Mac, Ctrl elsewhere) is required.
  meta?: boolean
}

// Document-level keyboard shortcut. The handler fires only when the
// user isn't typing in an editable element — except for keys that
// must work everywhere (Escape always fires).
export function useHotkey(spec: HotkeySpec, handler: (e: KeyboardEvent) => void) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const key = e.key.toLowerCase()
      if (key !== spec.key) return
      if (spec.meta) {
        // Cmd on Mac, Ctrl elsewhere.
        const meta = e.metaKey || e.ctrlKey
        if (!meta) return
      }
      // Escape always fires; other shortcuts skip when typing.
      if (key !== 'escape') {
        const target = e.target as HTMLElement | null
        if (target && (
          target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.isContentEditable
        )) return
      }
      e.preventDefault()
      handler(e)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [spec.key, spec.meta, handler])
}
