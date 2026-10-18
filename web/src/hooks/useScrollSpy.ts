import { useEffect, useState } from "react";

// useScrollSpy returns the id of the last section whose top has passed the
// trigger line near the top of the viewport, so exactly one id is always
// active. A heading clicked into view lands at its scroll-margin-top, which
// is above the trigger line, so it becomes the active section.
export function useScrollSpy(ids: string[], offsetTop = 120): string | null {
  const [activeId, setActiveId] = useState<string | null>(null);
  const idsKey = ids.join("|");

  useEffect(() => {
    if (ids.length === 0) return;

    let raf = 0;
    const update = () => {
      // A short last section can't scroll up to the trigger line, so the
      // bottom of the page selects it.
      const atBottom =
        window.scrollY + window.innerHeight >=
        document.documentElement.scrollHeight - 4;
      if (atBottom) {
        setActiveId(ids[ids.length - 1] ?? null);
        return;
      }

      let active: string | null = null;
      for (const id of ids) {
        const el = document.getElementById(id);
        if (!el) continue;
        if (el.getBoundingClientRect().top <= offsetTop) {
          active = id;
        } else {
          // ids are in document order.
          break;
        }
      }
      // Above the first section, the first id stays active.
      if (!active) active = ids[0] ?? null;
      setActiveId(active);
    };

    const onScroll = () => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        update();
      });
    };

    update();
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll);
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
      if (raf) cancelAnimationFrame(raf);
    };
    // idsKey stands in for the array so the listeners don't rebind each render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey, offsetTop]);

  // With no ids the listener never ran, so the stored id is stale.
  return ids.length === 0 ? null : activeId;
}
