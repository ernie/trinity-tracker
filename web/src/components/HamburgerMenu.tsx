import { useEffect, useRef, useState, type ReactNode } from "react";
import { useLocation } from "react-router-dom";

interface HamburgerMenuProps {
  children: ReactNode;
  /** Modifier classes for the root, such as 'hamburger--authed'. */
  className?: string;
}

// On desktop the wrapper is `display: contents`, so its children sit in
// the parent's flex layout; on mobile it becomes a button and popup.
export function HamburgerMenu({ children, className }: HamburgerMenuProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const location = useLocation();

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node))
        setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  // Closing during render keeps the menu from showing open for a frame
  // after a nav tap commits.
  const [navPath, setNavPath] = useState(location.pathname);
  if (location.pathname !== navPath) {
    setNavPath(location.pathname);
    setOpen(false);
  }

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  return (
    <div
      className={`hamburger ${open ? "hamburger--open" : ""} ${className ?? ""}`}
      ref={ref}
    >
      <button
        type="button"
        className="hamburger__btn"
        onClick={() => setOpen((v) => !v)}
        aria-label={open ? "Close menu" : "Open menu"}
        aria-expanded={open}
        aria-haspopup="menu"
      >
        <svg
          width="18"
          height="14"
          viewBox="0 0 18 14"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <line x1="2" y1="3" x2="16" y2="3" />
          <line x1="2" y1="7" x2="16" y2="7" />
          <line x1="2" y1="11" x2="16" y2="11" />
        </svg>
      </button>
      <div className="hamburger__popup" role="menu">
        {children}
      </div>
    </div>
  );
}
