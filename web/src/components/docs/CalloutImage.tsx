import {
  useState,
  useRef,
  useEffect,
  type ReactNode,
  type CSSProperties,
} from "react";

// One annotated region in a screenshot. xPct/yPct mark the top-left
// corner; wPct/hPct the size — all as percentages of the image's
// rendered dimensions, so the overlay scales correctly at any width.
// title/body match the legend list rendered alongside; the same data
// drives both surfaces.
export interface CalloutData {
  n: number;
  xPct: number;
  yPct: number;
  wPct: number;
  hPct: number;
  title: string;
  body: ReactNode;
}

interface CalloutImageProps {
  src: string;
  alt: string;
  callouts: CalloutData[];
}

// Collapsed-by-default numbered legend that pairs with a CalloutImage.
// Walking the highlighted regions on the image (hover / focus / tap) is
// the primary surface; the full legend is here as a fallback when
// readers want it as a list. Rendered as a native <details> so it gets
// keyboard handling and accessibility for free; the disclosure marker
// is a custom rotating chevron via CSS so it renders the same on every
// browser.
export function CalloutLegend({ callouts }: { callouts: CalloutData[] }) {
  return (
    <details className="docs-callout-legend">
      <summary>Hover/tap the highlighted areas, or expand for legend</summary>
      <ol className="docs-hud-legend">
        {callouts.map((c) => (
          <li key={c.n}>
            <strong>{c.title}.</strong> {c.body}
          </li>
        ))}
      </ol>
    </details>
  );
}

// Annotated screenshot with click-to-expand and hover/focus/tap-
// revealed tooltips. The overlay (hotspot buttons + active tooltip)
// renders both inline and inside the zoom modal — same React state,
// same JSX, just two parents — so a hotspot a user pinned inline
// stays pinned when they zoom in to inspect it more closely, and
// vice versa.
//
// CalloutImage is self-contained (does not delegate to ZoomableImage)
// so the zoom-modal stage can be sized to the image and host its own
// percentage-positioned overlay. Body-scroll lock, Esc-to-close, and
// focus restore are duplicated lightly from ZoomableImage rather
// than abstracted out — the duplication is small, self-contained,
// and keeps each component easy to reason about on its own.
//
// Pointer-events trick: the inline overlay container is
// pointer-events:none, hotspot buttons inside are pointer-events:auto.
// That lets clicks fall through to the trigger button underneath
// everywhere except over a hotspot. In the zoom modal, the stage
// stops click propagation so clicking the image area doesn't dismiss
// the modal — only the dark backdrop or the close button do.
export function CalloutImage({ src, alt, callouts }: CalloutImageProps) {
  const [activeN, setActiveN] = useState<number | null>(null);
  const [zoomed, setZoomed] = useState(false);
  const closeTimerRef = useRef<number | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  const show = (n: number) => {
    if (closeTimerRef.current !== null) {
      clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
    setActiveN(n);
  };

  // Small close delay bridges the gap between mouse leaving a hotspot
  // and entering the tooltip (which is positioned outside the
  // hotspot's bounds). Without this, moving the cursor onto the
  // tooltip flickers it off and on.
  const scheduleClose = () => {
    closeTimerRef.current = window.setTimeout(() => {
      setActiveN(null);
      closeTimerRef.current = null;
    }, 120);
  };

  const togglePinned = (n: number) => {
    if (activeN === n) {
      setActiveN(null);
    } else {
      show(n);
    }
  };

  // Esc dismisses the active tooltip first (if any), then the zoom
  // modal — the user's intuition is "Escape closes the topmost
  // thing." Keydown is wired on both the inline container and the
  // modal so it works in either context.
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "Escape") return;
    if (activeN !== null) {
      e.preventDefault();
      setActiveN(null);
    } else if (zoomed) {
      e.preventDefault();
      setZoomed(false);
    }
  };

  // Body-scroll lock + focus management for the zoom modal. Mirrors
  // ZoomableImage's effect; lives here because CalloutImage owns
  // its own modal. The document-level keydown picks up Escape when
  // focus isn't in our React tree (e.g., user clicked the modal
  // backdrop — focus is on body).
  useEffect(() => {
    if (!zoomed) return;
    // Snapshot the trigger at effect setup so cleanup restores focus
    // to *this* render's button, not whatever the ref happens to be
    // pointing at later (react-hooks/exhaustive-deps).
    const triggerEl = triggerRef.current;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && activeN === null) {
        setZoomed(false);
      }
    };
    document.addEventListener("keydown", onKey);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prevOverflow;
      triggerEl?.focus();
    };
  }, [zoomed, activeN]);

  const activeCallout =
    activeN !== null ? callouts.find((c) => c.n === activeN) : null;

  // Shared overlay JSX. Rendered into both the inline view and the
  // zoom modal — both invocations bind to the same React state, so
  // the active callout's tooltip follows whichever view is showing.
  const overlay = (
    <>
      {callouts.map((c) => (
        <button
          key={c.n}
          type="button"
          className={`docs-callout-image__hotspot ${activeN === c.n ? "is-active" : ""}`}
          style={{
            left: `${c.xPct}%`,
            top: `${c.yPct}%`,
            width: `${c.wPct}%`,
            height: `${c.hPct}%`,
          }}
          onMouseEnter={() => show(c.n)}
          onMouseLeave={scheduleClose}
          onFocus={() => show(c.n)}
          onBlur={scheduleClose}
          onClick={(e) => {
            // Stop propagation so clicks inside the zoom modal
            // don't bubble up to the backdrop and dismiss it.
            e.stopPropagation();
            togglePinned(c.n);
          }}
          aria-label={`Callout ${c.n}: ${c.title}`}
          aria-expanded={activeN === c.n}
        />
      ))}
      {activeCallout && (
        <Tooltip
          callout={activeCallout}
          onMouseEnter={() => show(activeCallout.n)}
          onMouseLeave={scheduleClose}
        />
      )}
    </>
  );

  return (
    <>
      <div className="docs-callout-image" onKeyDown={onKeyDown}>
        <button
          ref={triggerRef}
          type="button"
          className="docs-callout-image__trigger"
          onClick={() => setZoomed(true)}
          aria-label={`${alt} Click to view full size.`}
        >
          <img src={src} alt={alt} />
        </button>
        <div className="docs-callout-image__overlay">{overlay}</div>
      </div>
      {zoomed && (
        <div
          className="docs-callout-image__zoom"
          role="dialog"
          aria-modal="true"
          aria-label="Full-size image viewer"
          onClick={() => setZoomed(false)}
          onKeyDown={onKeyDown}
        >
          <div
            className="docs-callout-image__zoom-stage"
            onClick={(e) => e.stopPropagation()}
          >
            <img src={src} alt={alt} className="docs-callout-image__zoom-img" />
            <div className="docs-callout-image__overlay">{overlay}</div>
          </div>
          <button
            ref={closeRef}
            type="button"
            className="docs-callout-image__zoom-close"
            onClick={() => setZoomed(false)}
            aria-label="Close full-size view"
          >
            ×
          </button>
        </div>
      )}
    </>
  );
}

interface TooltipProps {
  callout: CalloutData;
  onMouseEnter: () => void;
  onMouseLeave: () => void;
}

// Tooltip placement is a two-axis heuristic on the callout's
// position within the image:
//
//   Horizontal: if the box's center sits in the outer third (≤30%
//   or ≥70%), anchor the tooltip's near edge to the box's near
//   edge — that keeps the tooltip inside image bounds even when
//   the box hugs an edge. Otherwise center on the box midpoint.
//
//   Vertical: if the box's bottom edge is in the upper 60% of the
//   image, place tooltip below; otherwise above. Avoids tooltips
//   running off the bottom for bottom-row callouts and off the top
//   for top-row callouts.
//
// Pure CSS — no JS measurement, no edge-detection on the rendered
// tooltip itself. Trade-off: tooltips with content wider than the
// available anchor-side space can still overflow the image's right
// or left edge, but only marginally compared to the previous
// always-centered approach.
function Tooltip({ callout: c, onMouseEnter, onMouseLeave }: TooltipProps) {
  const centerPct = c.xPct + c.wPct / 2;

  let horiz: CSSProperties;
  if (centerPct >= 70) {
    horiz = { right: `${100 - c.xPct - c.wPct}%` };
  } else if (centerPct <= 30) {
    horiz = { left: `${c.xPct}%` };
  } else {
    horiz = { left: `${centerPct}%`, transform: "translateX(-50%)" };
  }

  const placeBelow = c.yPct + c.hPct < 60;
  const vert: CSSProperties = placeBelow
    ? { top: `${c.yPct + c.hPct + 1}%` }
    : { bottom: `${100 - c.yPct + 1}%` };

  return (
    <div
      role="tooltip"
      className={`docs-callout-image__tooltip ${placeBelow ? "is-below" : "is-above"}`}
      style={{ ...horiz, ...vert }}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <strong>
        {c.n}. {c.title}.
      </strong>{" "}
      {c.body}
    </div>
  );
}
