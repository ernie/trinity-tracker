import { useState, useEffect, useRef } from "react";

interface ZoomableImageProps {
  src: string;
  alt: string;
}

// Opens to full size on click. The open view has one focusable
// element, the close button, so it needs no focus trap.
export function ZoomableImage({ src, alt }: ZoomableImageProps) {
  const [zoomed, setZoomed] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!zoomed) return;
    // Captured at setup so cleanup refocuses this render's trigger.
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
