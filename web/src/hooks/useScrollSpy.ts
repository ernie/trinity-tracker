import { useEffect, useState } from 'react'

// useScrollSpy returns the id of the section the reader is most likely
// looking at: the *last* one whose top edge has crossed above the
// trigger line near the top of the viewport. This always yields exactly
// one active id (no "gap" frames where nothing is highlighted), and it
// agrees with scroll-margin-top — when the user clicks an anchor, the
// destination heading lands at y = scroll-margin-top, which is below the
// trigger line, so it's correctly recognized as the new active section.
//
// We listen to scroll/resize via rAF rather than IntersectionObserver
// because the previous IO-based implementation had two problems:
//   1. Headings landing inside the rootMargin top-cutoff (e.g. 48px
//      from scroll-margin-top, with a 120px cutoff) were considered
//      "not visible" and the spy fell through to the *next* section.
//   2. Between two sections it was possible for neither to intersect
//      the narrow spy band, leaving the rail un-highlighted.
//
// rAF-throttled scroll handling costs effectively nothing while idle.
export function useScrollSpy(ids: string[], offsetTop = 120): string | null {
  const [activeId, setActiveId] = useState<string | null>(null)
  const idsKey = ids.join('|')

  useEffect(() => {
    if (ids.length === 0) return  // no sections to spy on

    let raf = 0
    const update = () => {
      // Edge case: when the user clicks the *last* anchor, the browser
      // can only scroll until `scrollY + innerHeight = scrollHeight`.
      // For a section near the bottom of the page, that maximum may
      // still leave its top below our trigger line, so the regular
      // "last past trigger" loop never selects it. Detect bottom-of-
      // page first and short-circuit to the last id.
      const atBottom =
        window.scrollY + window.innerHeight >=
        document.documentElement.scrollHeight - 4
      if (atBottom) {
        setActiveId(ids[ids.length - 1] ?? null)
        return
      }

      let active: string | null = null
      for (const id of ids) {
        const el = document.getElementById(id)
        if (!el) continue
        if (el.getBoundingClientRect().top <= offsetTop) {
          active = id
        } else {
          // ids are in document order; once we hit a section that hasn't
          // yet crossed the trigger, no later section has either.
          break
        }
      }
      // Before the first section reaches the trigger line, keep the
      // first one highlighted so the rail is never blank.
      if (!active) active = ids[0] ?? null
      setActiveId(active)
    }

    const onScroll = () => {
      if (raf) return
      raf = requestAnimationFrame(() => {
        raf = 0
        update()
      })
    }

    update()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
      if (raf) cancelAnimationFrame(raf)
    }
    // idsKey turns the array dep into a stable scalar so we don't
    // rebind listeners on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey, offsetTop])

  // When the caller has no ids to spy on, the listener didn't run, so
  // any value left in state is stale — return null so the consumer
  // doesn't act on it.
  return ids.length === 0 ? null : activeId
}
