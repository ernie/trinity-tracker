import { Link, useLocation } from "react-router-dom";

// Same order as the landing page doors.
const NAV_ITEMS = [
  { path: "/servers", label: "Servers" },
  { path: "/matches", label: "Matches" },
  { path: "/players", label: "Players" },
  { path: "/leaderboard", label: "Leaderboard" },
  { path: "/docs", label: "Docs" },
];

export function PageNav() {
  const location = useLocation();

  const isActive = (path: string) => {
    if (path === "/") return location.pathname === "/";
    if (path === "/docs") return location.pathname.startsWith("/docs");
    return location.pathname.startsWith(path);
  };

  return (
    <nav className="page-nav">
      {NAV_ITEMS.map((item) => {
        const active = isActive(item.path);
        return (
          <Link
            key={item.path}
            to={item.path}
            className={`nav-link ${active ? "active" : ""}`}
          >
            {/* Every link reserves the glyph's width so labels hold still. */}
            <img
              src="/assets/icon-128.png"
              alt=""
              aria-hidden="true"
              className={`nav-link__glyph ${active ? "" : "nav-link__glyph--placeholder"}`}
            />
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
