// Inside this root, any element with `data-help="..."` shows a popover on
// hover, focus or tap; elsewhere the attribute does nothing.
//
// `data-help` strings are plain text, and \n is a hard line break. Wrap
// long source lines with string concatenation so they add no breaks.
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

interface PopoverState {
  text: string;
  rect: DOMRect;
}

export function HelpModeRoot({ children }: { children: ReactNode }) {
  const [popover, setPopover] = useState<PopoverState | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;

    // e.target can be a Text node, the document, or null.
    const helpTargetFrom = (e: Event): HTMLElement | null => {
      const el = e.target as Element | null;
      if (!el || typeof el.closest !== "function") return null;
      return el.closest("[data-help]") as HTMLElement | null;
    };

    const show = (e: Event) => {
      const target = helpTargetFrom(e);
      if (!target) return;
      setPopover({
        text: target.getAttribute("data-help")!,
        rect: target.getBoundingClientRect(),
      });
    };

    // Moving between children of one described element keeps the popover.
    const hide = (e: Event) => {
      const related = (e as MouseEvent | FocusEvent)
        .relatedTarget as Element | null;
      if (
        related &&
        typeof related.closest === "function" &&
        related.closest("[data-help]")
      )
        return;
      setPopover(null);
    };

    // Heard at document level so a tap outside the root closes the
    // popover. Every root on the page hears each tap, so a closed one
    // keeps its state and skips the render.
    const tap = (e: Event) => {
      const target = helpTargetFrom(e);
      if (target && root.contains(target)) {
        setPopover({
          text: target.getAttribute("data-help")!,
          rect: target.getBoundingClientRect(),
        });
      } else {
        setPopover((prev) => (prev === null ? prev : null));
      }
    };

    // Described elements are inspect-only: cancel activation so a tap
    // on e.g. the LIVE pill shows its popover instead of navigating.
    const blockActivation = (e: Event) => {
      const target = helpTargetFrom(e);
      if (target && root.contains(target)) {
        e.preventDefault();
        e.stopPropagation();
      }
    };

    root.addEventListener("mouseover", show);
    root.addEventListener("mouseout", hide);
    root.addEventListener("focusin", show);
    root.addEventListener("focusout", hide);
    root.addEventListener("click", blockActivation, true);
    document.addEventListener("touchstart", tap, { passive: true });
    return () => {
      root.removeEventListener("mouseover", show);
      root.removeEventListener("mouseout", hide);
      root.removeEventListener("focusin", show);
      root.removeEventListener("focusout", hide);
      root.removeEventListener("click", blockActivation, true);
      document.removeEventListener("touchstart", tap);
    };
  }, []);

  return (
    <div ref={rootRef} className="help-mode-root">
      {children}
      {popover && <HelpPopover text={popover.text} anchorRect={popover.rect} />}
    </div>
  );
}

// Portaled to document.body to escape ancestor `overflow: hidden` and the
// cards' `.card > * { z-index: 1 }` stacking.
const SAFE_ABOVE_PX = 160;
const VIEWPORT_MARGIN = 8;
// Matches the CSS `max-width: min(320px, 80vw)`, so the horizontal clamp
// keeps the widest popover on screen.
function maxPopoverWidth() {
  return Math.min(320, window.innerWidth * 0.8);
}

function HelpPopover({
  text,
  anchorRect,
}: {
  text: string;
  anchorRect: DOMRect;
}) {
  // The first side comes from the anchor's position; the layout effect
  // measures the popover and flips it before paint if that side overflows.
  const ref = useRef<HTMLDivElement>(null);
  const [side, setSide] = useState<"above" | "below">(
    anchorRect.top > SAFE_ABOVE_PX ? "above" : "below",
  );
  const adjustedRef = useRef(false);

  useLayoutEffect(() => {
    if (!ref.current || adjustedRef.current) return;
    const rect = ref.current.getBoundingClientRect();
    adjustedRef.current = true;
    if (side === "above" && rect.top < VIEWPORT_MARGIN) {
      setSide("below");
    } else if (
      side === "below" &&
      rect.bottom > window.innerHeight - VIEWPORT_MARGIN
    ) {
      if (anchorRect.top - rect.height - VIEWPORT_MARGIN * 2 > 0) {
        setSide("above");
      }
    }
  }, [side, anchorRect]);

  const halfWidth = maxPopoverWidth() / 2;
  const minCenter = halfWidth + VIEWPORT_MARGIN;
  const maxCenter = window.innerWidth - halfWidth - VIEWPORT_MARGIN;
  const rawCenter = anchorRect.left + anchorRect.width / 2;
  const centerX = Math.max(minCenter, Math.min(maxCenter, rawCenter));
  const style: React.CSSProperties =
    side === "above"
      ? {
          left: centerX,
          top: anchorRect.top - 8,
          transform: "translate(-50%, -100%)",
        }
      : {
          left: centerX,
          top: anchorRect.bottom + 8,
          transform: "translateX(-50%)",
        };

  return createPortal(
    <div
      ref={ref}
      className={`help-popover help-popover--${side}`}
      role="tooltip"
      style={style}
    >
      <div className="help-popover__text">{text}</div>
    </div>,
    document.body,
  );
}
