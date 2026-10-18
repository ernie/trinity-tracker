import { Link, useLocation } from "react-router-dom";
import { DOCS_TABS } from "./DocsTocRail";
import { ArrowIcon } from "../ArrowIcon";

// Welcome's empty path is matched exactly, since a prefix match on
// /docs/ would match every page.
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
