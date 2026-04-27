import { NavLink, Navigate, Outlet } from 'react-router-dom'
import { Header } from '../Header'
import { useAuth } from '../../hooks/useAuth'

const ADMIN_TABS = [
  { path: 'users', label: 'Users' },
  { path: 'sessions', label: 'Sessions' },
  { path: 'players', label: 'Players' },
  { path: 'sources', label: 'Sources' },
]

export function AdminPage() {
  const { auth, loading } = useAuth()

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
