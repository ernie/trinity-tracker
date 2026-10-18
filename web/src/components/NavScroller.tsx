import { useEffect, useRef, useState, type ReactNode } from "react";

interface NavScrollerProps {
  children: ReactNode;
  // Each nav brings its own scroll class for its mobile overflow and mask
  // CSS; the is-scroll-left and is-scroll-right classes join it.
  scrollClassName?: string;
}

// On desktop the wrapper is `display: contents`; on mobile it positions
// the edge chevrons.
export function NavScroller({
  children,
  scrollClassName = "app-header__nav-scroll",
}: NavScrollerProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const update = () => {
      const { scrollLeft, scrollWidth, clientWidth } = el;
      // 1px tolerance absorbs sub-pixel widths and scroll snap residue.
      setCanScrollLeft(scrollLeft > 1);
      setCanScrollRight(scrollLeft + clientWidth < scrollWidth - 1);
    };
    update();
    el.addEventListener("scroll", update, { passive: true });
    // The nav's width changes with rotation and with login state.
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => {
      el.removeEventListener("scroll", update);
      ro.disconnect();
    };
  }, []);

  // The bar always shows at least half the items, so one jump to the
  // edge reveals the rest.
  const scrollToEdge = (direction: "left" | "right") => {
    const el = scrollRef.current;
    if (!el) return;
    const target = direction === "left" ? 0 : el.scrollWidth - el.clientWidth;
    el.scrollTo({ left: target, behavior: "smooth" });
  };

  const scrollClass = [
    scrollClassName,
    canScrollLeft ? "is-scroll-left" : "",
    canScrollRight ? "is-scroll-right" : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className="nav-scroller">
      <div className={scrollClass} ref={scrollRef}>
        {children}
      </div>
      <button
        type="button"
        className={`nav-scroller__edge nav-scroller__edge--left ${canScrollLeft ? "is-shown" : ""}`}
        onClick={() => scrollToEdge("left")}
        tabIndex={canScrollLeft ? 0 : -1}
        aria-label="Scroll navigation to start"
        aria-hidden={!canScrollLeft}
      >
        <svg
          width="8"
          height="12"
          viewBox="0 0 8 12"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <polyline points="5.5,2 2,6 5.5,10" />
        </svg>
      </button>
      <button
        type="button"
        className={`nav-scroller__edge nav-scroller__edge--right ${canScrollRight ? "is-shown" : ""}`}
        onClick={() => scrollToEdge("right")}
        tabIndex={canScrollRight ? 0 : -1}
        aria-label="Scroll navigation to end"
        aria-hidden={!canScrollRight}
      >
        <svg
          width="8"
          height="12"
          viewBox="0 0 8 12"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <polyline points="2.5,2 6,6 2.5,10" />
        </svg>
      </button>
    </div>
  );
}
