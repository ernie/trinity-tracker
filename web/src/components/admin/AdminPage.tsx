import { useEffect, useState } from "react";
import { NavLink, Navigate, Outlet } from "react-router-dom";
import { useAuth } from "../../hooks/useAuth";
import { NavScroller } from "../NavScroller";

const ADMIN_TABS = [
  { path: "users", label: "Users" },
  { path: "sessions", label: "Sessions" },
  { path: "players", label: "Players" },
  { path: "sources", label: "Sources" },
  { path: "audit", label: "Audit" },
] as const;

export function AdminPage() {
  const { auth, loading } = useAuth();
  const [pendingCount, setPendingCount] = useState(0);

  useEffect(() => {
    if (!auth.isAuthenticated || !auth.isAdmin || !auth.token) return;
    let cancelled = false;
    const tick = () => {
      fetch("/api/admin/sources/pending", {
        headers: { Authorization: `Bearer ${auth.token}` },
      })
        .then((r) => (r.ok ? r.json() : []))
        .then((rows: unknown[]) => {
          if (!cancelled)
            setPendingCount(Array.isArray(rows) ? rows.length : 0);
        })
        .catch(() => {});
    };
    tick();
    const id = setInterval(tick, 30_000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [auth.isAuthenticated, auth.isAdmin, auth.token]);

  if (loading) {
    return (
      <div className="admin-page">
        <div className="admin-loading">Loading…</div>
      </div>
    );
  }

  if (!auth.isAuthenticated || !auth.isAdmin) {
    return <Navigate to="/" replace />;
  }

  return (
    <div className="admin-page">
      {/* .admin-workspace owns the warm editorial hairline that marks
          entry into the admin surface. One use per page, kept restrained
          so the gradient stays meaningful. */}
      <div className="admin-workspace">
        <div className="admin-layout">
          <nav className="admin-sidebar">
            {/* On desktop both NavScroller's wrapper and the strip below
                are display: contents, so links sit directly in the
                sidebar's flex column. On mobile the wrapper hosts the
                edge chevrons (absolute) and the strip becomes a flex
                row sized to its content (width: max-content), with the
                outer .admin-sidebar__scroll providing overflow + masks.
                Same shape as PageNav inside the header NavScroller. */}
            <NavScroller scrollClassName="admin-sidebar__scroll">
              <div className="admin-sidebar__strip">
                {ADMIN_TABS.map((tab) => (
                  <NavLink
                    key={tab.path}
                    to={`/admin/${tab.path}`}
                    className={({ isActive }) =>
                      `admin-sidebar-link ${isActive ? "active" : ""}`
                    }
                  >
                    <span>{tab.label}</span>
                    {tab.path === "sources" && pendingCount > 0 && (
                      <span
                        className="admin-sidebar-link__badge"
                        title={`${pendingCount} pending requests`}
                      >
                        {pendingCount}
                      </span>
                    )}
                  </NavLink>
                ))}
              </div>
            </NavScroller>
          </nav>

          <div className="admin-content">
            <Outlet />
          </div>
        </div>
      </div>
    </div>
  );
}
