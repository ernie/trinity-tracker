import { useEffect, useRef, useState } from "react";
import { useLocation, Outlet } from "react-router-dom";
import { Header } from "./Header";
import { DocsTocRail, type DocSection } from "./docs/DocsTocRail";
import { DocsOnThisPage } from "./docs/DocsOnThisPage";
import { DocsPrevNext } from "./docs/DocsPrevNext";

export function DocsPage() {
  const location = useLocation();
  const contentRef = useRef<HTMLDivElement>(null);
  const [sections, setSections] = useState<DocSection[]>([]);

  // Re-scan headings whenever the docs subroute changes. The Outlet
  // mounts the new subpage; rAF lets the DOM settle before we read it.
  useEffect(() => {
    let raf = 0;
    const scan = () => {
      const nodes = contentRef.current?.querySelectorAll('h2[id]') ?? [];
      const next: DocSection[] = [];
      nodes.forEach((n) => {
        if (n instanceof HTMLElement && n.id) {
          // Pull the visible text without the copy-anchor glyph.
          const textEl = n.querySelector('.docs-h2__text');
          const label = (textEl?.textContent ?? n.textContent ?? '').trim();
          next.push({ id: n.id, label });
        }
      });
      setSections(next);
    };
    raf = requestAnimationFrame(scan);
    return () => cancelAnimationFrame(raf);
  }, [location.pathname]);

  // Route changes inside the docs (prev/next, TOC clicks) need explicit
  // scroll handling — React Router doesn't reset scroll, so without this
  // a click on "Next" leaves the viewport at the bottom of the new page
  // staring at its prev/next bar. With a hash, scroll to the anchor;
  // without one, jump to the top of the new page.
  useEffect(() => {
    const tryScroll = () => {
      if (location.hash) {
        const el = document.getElementById(location.hash.slice(1));
        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
      } else {
        window.scrollTo({ top: 0, behavior: 'auto' });
      }
    };
    const t = setTimeout(tryScroll, 50);
    return () => clearTimeout(t);
  }, [location.pathname, location.hash]);

  return (
    <div className="about-page docs-page-v2">
      <Header title="Docs" className="about-header" />

      <div className="docs-layout">
        <aside className="docs-layout__left">
          <DocsTocRail />
        </aside>

        <main className="docs-layout__content" ref={contentRef}>
          <Outlet />
          <DocsPrevNext />

          <div className="about-section docs-disclaimer">
            <h2>Please don't sue me.</h2>
            <p>
              Trinity is not affiliated, associated, authorized, endorsed by, or
              in any way officially connected with Bethesda or id Software, or any
              of its subsidiaries or its affiliates. Quake 3, Quake 3 Arena, id,
              id Software, id Tech and related logos are registered trademarks or
              trademarks of id Software LLC in the U.S. and/or other countries.
              All Rights Reserved.
            </p>
          </div>
        </main>

        <aside className="docs-layout__right">
          <DocsOnThisPage sections={sections} />
        </aside>
      </div>
    </div>
  );
}
