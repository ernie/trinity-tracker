import { useEffect, useState } from 'react'
import { NavLink, Navigate, Outlet } from 'react-router-dom'
import { Header } from '../Header'
import { useAuth } from '../../hooks/useAuth'

const ADMIN_TABS = [
  { path: 'users', label: 'Users' },
  { path: 'sessions', label: 'Sessions' },
  { path: 'players', label: 'Players' },
  { path: 'sources', label: 'Sources' },
] as const

export function AdminPage() {
  const { auth, loading } = useAuth()
  const [pendingCount, setPendingCount] = useState(0)

  useEffect(() => {
    if (!auth.isAuthenticated || !auth.isAdmin || !auth.token) return
    let cancelled = false
    const tick = () => {
      fetch('/api/admin/sources/pending', {
        headers: { Authorization: `Bearer ${auth.token}` },
      })
        .then((r) => (r.ok ? r.json() : []))
        .then((rows: unknown[]) => {
          if (!cancelled) setPendingCount(Array.isArray(rows) ? rows.length : 0)
        })
        .catch(() => {})
    }
    tick()
    const id = setInterval(tick, 30_000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [auth.isAuthenticated, auth.isAdmin, auth.token])

  if (loading) {
    return (
      <div className="admin-page">
        <Header title="Admin" className="admin-header" />
        <div className="admin-loading">Loading…</div>
      </div>
    )
  }

  if (!auth.isAuthenticated || !auth.isAdmin) {
    return <Navigate to="/" replace />
  }

  return (
    <div className="admin-page">
      <Header title="Admin" className="admin-header" />

      <div className="admin-layout">
        <nav className="admin-sidebar">
          {ADMIN_TABS.map((tab) => (
            <NavLink
              key={tab.path}
              to={`/admin/${tab.path}`}
              className={({ isActive }) =>
                `admin-sidebar-link ${isActive ? 'active' : ''}`
              }
            >
              {tab.label}
              {tab.path === 'sources' && pendingCount > 0 && (
                <span className="admin-nav-badge"> ({pendingCount} pending)</span>
              )}
            </NavLink>
          ))}
        </nav>

        <div className="admin-content">
          <Outlet />
        </div>
      </div>
    </div>
  )
}
