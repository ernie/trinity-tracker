import { useEffect, useRef, useState, type RefObject } from "react";
import { useLocation, Outlet } from "react-router-dom";
import { DocsTocRail, type DocSection } from "./docs/DocsTocRail";
import { DocsOnThisPage } from "./docs/DocsOnThisPage";
import { DocsPrevNext } from "./docs/DocsPrevNext";
import { PlatformProvider, usePlatform } from "./docs/PlatformContext";
import { PlatformPicker } from "./docs/PlatformPicker";
import { PlatformBadge } from "./docs/PlatformBadge";

// Platform-only sections come and go with the platform, so the headings
// are rescanned on a platform change too, a frame after the Outlet mounts.
function DocsSectionsRail({
  contentRef,
}: {
  contentRef: RefObject<HTMLDivElement | null>;
}) {
  const { pathname } = useLocation();
  const { platform } = usePlatform();
  const [sections, setSections] = useState<DocSection[]>([]);

  useEffect(() => {
    const raf = requestAnimationFrame(() => {
      const nodes = contentRef.current?.querySelectorAll("h2[id]") ?? [];
      const next: DocSection[] = [];
      nodes.forEach((n) => {
        if (n instanceof HTMLElement && n.id) {
          const textEl = n.querySelector(".docs-h2__text");
          const label = (textEl?.textContent ?? n.textContent ?? "").trim();
          next.push({ id: n.id, label });
        }
      });
      setSections(next);
    });
    return () => cancelAnimationFrame(raf);
  }, [contentRef, pathname, platform]);

  return <DocsOnThisPage sections={sections} />;
}

export function DocsPage() {
  const location = useLocation();
  const contentRef = useRef<HTMLDivElement>(null);

  // React Router keeps the scroll position across docs route changes.
  useEffect(() => {
    const tryScroll = () => {
      if (location.hash) {
        const el = document.getElementById(location.hash.slice(1));
        if (el) el.scrollIntoView({ behavior: "smooth", block: "start" });
      } else {
        window.scrollTo({ top: 0, behavior: "auto" });
      }
    };
    const t = setTimeout(tryScroll, 50);
    return () => clearTimeout(t);
  }, [location.pathname, location.hash]);

  return (
    <PlatformProvider
      renderPicker={(onPick) => <PlatformPicker onPick={onPick} />}
    >
      <div className="about-page docs-page-v2">
        <div className="docs-layout">
          <aside className="docs-layout__left">
            <PlatformBadge />
            <DocsTocRail />
          </aside>

          <main className="docs-layout__content" ref={contentRef}>
            <Outlet />
            <DocsPrevNext />
          </main>

          <aside className="docs-layout__right">
            <DocsSectionsRail contentRef={contentRef} />
          </aside>
        </div>
      </div>
    </PlatformProvider>
  );
}
