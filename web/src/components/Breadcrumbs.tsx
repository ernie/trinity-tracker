import { Link } from "react-router-dom";
import type { ReactNode } from "react";

export interface Crumb {
  label: ReactNode;
  to?: string;
}

interface BreadcrumbsProps {
  crumbs: Crumb[];
  className?: string;
}

// Simple breadcrumb trail. The last crumb renders as plain text (the
// "you are here" indicator); earlier crumbs are links.
export function Breadcrumbs({ crumbs, className }: BreadcrumbsProps) {
  if (crumbs.length === 0) return null;

  return (
    <nav className={`breadcrumbs ${className ?? ""}`} aria-label="Breadcrumb">
      <ol className="breadcrumbs__list">
        {crumbs.map((crumb, i) => {
          const isLast = i === crumbs.length - 1;
          return (
            <li key={i} className="breadcrumbs__item">
              {isLast || !crumb.to ? (
                <span
                  className="breadcrumbs__current"
                  aria-current={isLast ? "page" : undefined}
                >
                  {crumb.label}
                </span>
              ) : (
                <Link to={crumb.to} className="breadcrumbs__link">
                  {crumb.label}
                </Link>
              )}
              {!isLast && (
                <span className="breadcrumbs__sep" aria-hidden="true">
                  /
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
