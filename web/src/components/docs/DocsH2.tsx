import { useState, type ReactNode } from 'react'

interface DocsH2Props {
  id: string
  children: ReactNode
}

// h2 with an id and a hover-revealed copy-anchor link. Clicking the
// anchor copies a deep link to this section to the clipboard.
export function DocsH2({ id, children }: DocsH2Props) {
  const [copied, setCopied] = useState(false)

  const copyAnchor = () => {
    const url = `${window.location.origin}${window.location.pathname}#${id}`
    void navigator.clipboard.writeText(url).then(
      () => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      },
      () => { /* ignore — older browsers without clipboard API */ },
    )
  }

  return (
    <h2 id={id} className="docs-h2">
      <span className="docs-h2__text">{children}</span>
      <button
        type="button"
        className="docs-h2__anchor"
        title={copied ? 'Copied!' : 'Copy link to this section'}
        aria-label={`Copy link to ${typeof children === 'string' ? children : 'this section'}`}
        onClick={copyAnchor}
      >
        {copied ? '✓' : '#'}
      </button>
    </h2>
  )
}
