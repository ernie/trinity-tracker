import { Link, useLocation } from "react-router-dom";
import { DOCS_TABS } from "./DocsTocRail";
import { ArrowIcon } from "../ArrowIcon";

// Bottom-of-page sibling nav. Encourages reading flow through the
// docs as if it were a manual: ← prev tab · next tab →. Welcome
// (empty path) is matched by exact equality — `startsWith` against
// `/docs/` would match every sub-route and short-circuit the lookup
// to index 0, making Next on /docs/install point back to Install.
export function DocsPrevNext() {
  const location = useLocation();
  const idx = DOCS_TABS.findIndex((t) =>
    t.path === ""
      ? location.pathname === "/docs" || location.pathname === "/docs/"
      : location.pathname.startsWith(`/docs/${t.path}`),
  );
  if (idx === -1) return null;

  const prev = idx > 0 ? DOCS_TABS[idx - 1] : null;
  const next = idx < DOCS_TABS.length - 1 ? DOCS_TABS[idx + 1] : null;

  if (!prev && !next) return null;

  return (
    <nav className="docs-prev-next" aria-label="Documentation pagination">
      {prev ? (
        <Link
          to={`/docs/${prev.path}`}
          className="docs-prev-next__link docs-prev-next__link--prev"
        >
          <span className="docs-prev-next__direction">
            <ArrowIcon direction="left" /> Previous
          </span>
          <span className="docs-prev-next__label">{prev.label}</span>
        </Link>
      ) : (
        <span />
      )}
      {next ? (
        <Link
          to={`/docs/${next.path}`}
          className="docs-prev-next__link docs-prev-next__link--next"
        >
          <span className="docs-prev-next__direction">
            Next <ArrowIcon direction="right" />
          </span>
          <span className="docs-prev-next__label">{next.label}</span>
        </Link>
      ) : (
        <span />
      )}
    </nav>
  );
}
