import {
  useState,
  useRef,
  useEffect,
  type ReactNode,
  type CSSProperties,
} from "react";

// xPct/yPct are the top-left corner and wPct/hPct the size, as
// percentages of the rendered image, so the overlay scales with it.
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

// The same callouts as a list, for readers who'd rather read than
// explore the image.
export function CalloutLegend({ callouts }: { callouts: CalloutData[] }) {
  return (
    <details className="docs-callout-legend">
      <summary>
        Hover over or tap a highlighted area, or open the legend
      </summary>
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

// Owns its zoom view so the zoom stage can be sized to the image and
// carry the same percentage overlay. The overlay is pointer-events:
// none with hotspots set to auto, so a click anywhere else reaches the
// zoom trigger underneath.
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

  // The delay lets the cursor cross from a hotspot to its tooltip,
  // which sits outside the hotspot.
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

  // Escape closes the tooltip first, then the zoom view.
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

  // The document listener catches Escape when focus has left the
  // component, such as after a click on the backdrop.
  useEffect(() => {
    if (!zoomed) return;
    // Captured at setup so cleanup refocuses this render's trigger.
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

  // Rendered in both views from the same state, so a pinned callout
  // stays pinned when the reader zooms in or out.
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
            // Keeps a click in the zoom view from closing it.
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

// Placement keeps the tooltip on the image without measuring it: a
// box near either side anchors the tooltip to its near edge, and a box
// in the lower part of the image gets its tooltip above. A very wide
// tooltip can still run slightly past an edge.
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
