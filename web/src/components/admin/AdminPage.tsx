import { useEffect, useState } from "react";
import { NavLink, Navigate, Outlet } from "react-router-dom";
import { useAuth } from "../../hooks/useAuth";
import { NavScroller } from "../NavScroller";
import { apiFetch } from "../../authFetch";

const ADMIN_TABS = [
  { path: "users", label: "Users" },
  { path: "sessions", label: "Sessions" },
  { path: "platforms", label: "Platforms" },
  { path: "players", label: "Players" },
  { path: "sources", label: "Sources" },
  { path: "featured", label: "Featured" },
  { path: "audit", label: "Audit" },
] as const;

export function AdminPage() {
  const { auth, loading } = useAuth();
  const [pendingCount, setPendingCount] = useState(0);

  useEffect(() => {
    if (!auth.isAuthenticated || !auth.isAdmin) return;
    let cancelled = false;
    const tick = () => {
      apiFetch("/api/admin/sources/pending")
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
  }, [auth.isAuthenticated, auth.isAdmin]);

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
      <div className="admin-workspace">
        <div className="admin-layout">
          <nav className="admin-sidebar">
            {/* Same shape as PageNav in the header: display: contents on
                desktop, a scrolling strip with edge chevrons on mobile. */}
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
                        title={`${pendingCount} pending ${pendingCount === 1 ? "request" : "requests"}`}
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
