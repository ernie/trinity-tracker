import { useState, useEffect, useRef } from "react";

interface ZoomableImageProps {
  src: string;
  alt: string;
}

// An image that opens to full size in a backdrop overlay on click.
// Used in /docs/play for the annotated HUD / scoreboard screenshots,
// where the inline render at the docs content width (~600px) makes the
// numbered callouts hard to read in fine detail.
//
// Dismissal: click anywhere on the overlay (including the image), the
// close button, or press Escape. Body scroll is locked while open and
// focus moves to the close button; focus returns to the trigger on
// close. No focus trap beyond that — there's only one focusable element
// in the open state.
export function ZoomableImage({ src, alt }: ZoomableImageProps) {
  const [zoomed, setZoomed] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  // Effect handles the side-effects of the open state — keyboard
  // listener for Escape, body scroll lock, and focus management. All
  // three are torn down together when zoomed flips back to false.
  useEffect(() => {
    if (!zoomed) return;
    // Snapshot the trigger at setup so cleanup restores focus to the
    // button this effect "saw," not whatever the ref later points at
    // (react-hooks/exhaustive-deps).
    const triggerEl = triggerRef.current;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setZoomed(false);
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
  }, [zoomed]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className="docs-zoomable-image__trigger"
        onClick={() => setZoomed(true)}
        aria-label={`${alt} Click to view full size.`}
      >
        <img src={src} alt={alt} />
      </button>
      {zoomed && (
        <div
          className="docs-zoomable-image__overlay"
          role="dialog"
          aria-modal="true"
          aria-label="Full-size image viewer"
          onClick={() => setZoomed(false)}
        >
          <img src={src} alt={alt} className="docs-zoomable-image__full" />
          <button
            ref={closeRef}
            type="button"
            className="docs-zoomable-image__close"
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
