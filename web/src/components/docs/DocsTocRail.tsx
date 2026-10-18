import { useEffect, useRef } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { NavScroller } from "../NavScroller";

export interface DocSection {
  id: string;
  label: string;
}

// In reading order: install, play, set up an account, customize, run a
// server, look things up. Welcome's empty path is the /docs index.
export const DOCS_TABS = [
  { path: "", label: "Welcome" },
  { path: "install", label: "Install" },
  { path: "play", label: "Play" },
  { path: "account", label: "Account" },
  { path: "customize", label: "Customize" },
  { path: "admin", label: "Server admin" },
  { path: "reference", label: "Reference" },
] as const;

export function DocsTocRail() {
  const location = useLocation();
  const navRef = useRef<HTMLElement>(null);

  // On a phone the rail scrolls sideways, and a direct link can leave
  // the active tab off-screen.
  useEffect(() => {
    const active = navRef.current?.querySelector(
      ".docs-toc-rail__item--active",
    );
    if (active instanceof HTMLElement) {
      active.scrollIntoView({
        inline: "center",
        block: "nearest",
        behavior: "smooth",
      });
    }
  }, [location.pathname]);

  return (
    <nav
      ref={navRef}
      className="docs-toc-rail"
      aria-label="Documentation sections"
    >
      <div className="docs-toc-rail__title">Docs</div>
      <NavScroller scrollClassName="docs-toc-rail__scroll">
        <ul className="docs-toc-rail__list">
          {DOCS_TABS.map((tab) => {
            const isActive =
              tab.path === ""
                ? location.pathname === "/docs" ||
                  location.pathname === "/docs/"
                : location.pathname.startsWith(`/docs/${tab.path}`);
            return (
              <li
                key={tab.path}
                className={`docs-toc-rail__item ${isActive ? "docs-toc-rail__item--active" : ""}`}
              >
                <NavLink
                  to={tab.path === "" ? "/docs" : `/docs/${tab.path}`}
                  className="docs-toc-rail__tab"
                >
                  {tab.label}
                </NavLink>
              </li>
            );
          })}
        </ul>
      </NavScroller>
    </nav>
  );
}
