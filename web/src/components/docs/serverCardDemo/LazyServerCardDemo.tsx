// LazyServerCardDemo — defers mounting the actual ServerCardDemo
// until its placeholder scrolls within ~one viewport of being visible.
// On a long docs page with N demo cards (one per mode), mounting all
// of them eagerly multiplies the active DOM, event listeners, and
// React subtree weight — which on memory-constrained mobile Safari
// can compound into a tab crash. With lazy mount, steady state is
// one or two cards active rather than all six.
//
// The placeholder reserves vertical space so the page layout doesn't
// jump when a card mounts. minHeight is a coarse estimate; once
// mounted the card determines its own height.
import { useEffect, useRef, useState } from 'react'
import type { ServerStatus } from '../../../types'
import { ServerCardDemo } from './ServerCardDemo'

interface LazyServerCardDemoProps {
  snapshot: ServerStatus
  attackersOverride?: Set<number>
}

export function LazyServerCardDemo({ snapshot, attackersOverride }: LazyServerCardDemoProps) {
  const [visible, setVisible] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!ref.current) return
    const observer = new IntersectionObserver(
      ([entry]) => setVisible(entry.isIntersecting),
      // Mount ~half a viewport ahead of scroll; unmount when scrolled
      // farther than that. Symmetric mount/unmount keeps steady-state
      // weight bounded — without it, cards scrolled past stay alive
      // in memory and compound with other heavyweight page content
      // (autoplay videos, images) until mobile Safari kills the tab.
      { rootMargin: '50% 0px' },
    )
    observer.observe(ref.current)
    return () => observer.disconnect()
  }, [])

  return (
    <div ref={ref} className="lazy-server-card-demo" style={{ minHeight: visible ? undefined : '360px' }}>
      {visible && <ServerCardDemo snapshot={snapshot} attackersOverride={attackersOverride} />}
    </div>
  )
}
