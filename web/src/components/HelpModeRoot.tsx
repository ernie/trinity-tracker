// HelpModeRoot — the consumer for the `data-help` attribute convention.
// Wrap a subtree (e.g. a docs server-card demo) and any descendant with
// `data-help="..."` shows a popup on hover / focus / tap. Outside this
// component, `data-help` is inert: the leaf components publish the
// attribute, this root is the only thing that does anything with it.
//
// Authoring rule for `data-help` strings:
//   - Plain text only (no markdown, no HTML, no JSX).
//   - \n is a hard line break (rendered via `white-space: pre-line`).
//   - Template literals by default; use string concatenation to wrap
//     source for readability without injecting line breaks.
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

    // Resolve the nearest [data-help] ancestor from an event target.
    // Null-safe: e.target can be a Text node (no .closest), the
    // document itself, or null for synthesized events — return null
    // in all those cases without throwing.
    const helpTargetFrom = (e: Event): HTMLElement | null => {
      const el = e.target as Element | null;
      if (!el || typeof el.closest !== "function") return null;
      return el.closest("[data-help]") as HTMLElement | null;
    };

    // mouseover/mouseout bubble through descendants; resolve the
    // nearest [data-help] ancestor at event time so the leaf can be
    // any depth inside its describable element.
    const show = (e: Event) => {
      const target = helpTargetFrom(e);
      if (!target) return;
      setPopover({
        text: target.getAttribute("data-help")!,
        rect: target.getBoundingClientRect(),
      });
    };

    // Only hide when we're truly leaving a described region — moving
    // between two children of the same [data-help] element should not
    // flicker the popover off and back on.
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

    // Touch: first tap on a [data-help] opens; tap elsewhere closes.
    // Listens at document level so taps outside the root also dismiss
    // — but bail fast if there's nothing for THIS root to do, so
    // multi-root pages don't fan every tap into N redundant setStates.
    const tap = (e: Event) => {
      const target = helpTargetFrom(e);
      if (target && root.contains(target)) {
        setPopover({
          text: target.getAttribute("data-help")!,
          rect: target.getBoundingClientRect(),
        });
      } else {
        // Skip the setState entirely if we already have no popover.
        // Other HelpModeRoots on the page may also be handling this
        // tap; only the one with state change needs to re-render.
        setPopover((prev) => (prev === null ? prev : null));
      }
    };

    root.addEventListener("mouseover", show);
    root.addEventListener("mouseout", hide);
    root.addEventListener("focusin", show);
    root.addEventListener("focusout", hide);
    document.addEventListener("touchstart", tap, { passive: true });
    return () => {
      root.removeEventListener("mouseover", show);
      root.removeEventListener("mouseout", hide);
      root.removeEventListener("focusin", show);
      root.removeEventListener("focusout", hide);
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

// HelpPopover renders into a portal at document.body so it escapes
// ancestor `overflow: hidden` and stacking-context constraints (cards
// have `.card > * { z-index: 1 }` rules that would otherwise paint
// the popover behind card internals).
//
// Position decision: above the anchor by default; below if there's
// not enough room above. We don't measure the popover height — the
// `safeAbove` threshold is a static estimate that's right ~95% of
// the time for the short tooltips this scope produces. If a tooltip
// is so long it overflows the threshold, the editorial pass should
// shorten it, not the positioning code.
const SAFE_ABOVE_PX = 160;
const VIEWPORT_MARGIN = 8;
// Worst-case popover width, mirroring CSS `max-width: min(320px, 80vw)`.
// Used to clamp horizontal positioning so the popover never spills off
// a viewport edge on narrow screens. The actual rendered width may be
// less; the clamp is conservative (shift a few px more than strictly
// needed), which is invisible to the user.
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
  // Initial side guess is anchor-position-based — works for short
  // tooltips. For tall tooltips (multi-line bullet lists, etc.) the
  // useLayoutEffect below measures the rendered popover and flips
  // side if "above" would overflow the viewport top (or "below"
  // would overflow the bottom). The flip causes one re-render but
  // happens synchronously before paint, so the user never sees the
  // off-screen version.
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
      // Only flip to "above" if there's actually room there.
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
