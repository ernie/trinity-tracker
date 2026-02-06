import { useState } from 'react'
import { Link } from 'react-router-dom'
import { AppLogo } from './AppLogo'
import { PageNav } from './PageNav'
import { LoginForm } from './LoginForm'
import { UserManagement } from './UserManagement'
import { useAuth } from '../hooks/useAuth'

export function AboutPage() {
  const { auth, login, logout } = useAuth()
  const [showUserManagement, setShowUserManagement] = useState(false)

  return (
    <div className="about-page">
      <header className="about-header">
        <h1>
          <AppLogo />
          About
        </h1>
        <PageNav />
        <div className="auth-section">
          {auth.isAuthenticated ? (
            <div className="user-info">
              <Link to="/account" className="username-link">{auth.username}</Link>
              {auth.isAdmin && (
                <button onClick={() => setShowUserManagement(true)} className="admin-btn">Users</button>
              )}
              <button onClick={logout} className="logout-btn">Logout</button>
            </div>
          ) : (
            <LoginForm onLogin={(username, password) => login({ username, password })} />
          )}
        </div>
      </header>

      <div className="about-content">
        <div className="about-section">
          <h2>Built by</h2>
          <p>NilClass (<a href="https://ernie.io">ernie.io</a>, <a href="https://github.com/ernie">@ernie on GitHub</a>)</p>
        </div>

        <div className="about-section">
          <h2>The Project</h2>
          <p>A love letter to id Software games, especially Quake 3 Arena. Focused on the Quake 3 VR player community.</p>
        </div>

        <div className="about-section">
          <h2>Powered by</h2>
          <ul>
            <li><a href="https://github.com/ernie/trinity">trinity</a> &mdash; custom Quake 3 mod</li>
            <li><a href="https://github.com/ernie/Quake3e">Quake3e</a> &mdash; custom build of Quake3e</li>
            <li><a href="https://github.com/ernie/trinity-tools">trinity-tools</a> &mdash; this stats tracker</li>
          </ul>
        </div>

        <div className="about-section">
          <h2>Linking Accounts</h2>
          <p>Players can link multiple GUIDs to a single account using the <code>!link</code> command in-game. To get set up with an account, reach out via Discord (see below).</p>
        </div>

        <div className="about-section">
          <h2>Leaderboards</h2>
          <p>To appear on the leaderboards, players must set a custom name (no default "[VR] Player#nnnn" names) and complete at least 5 matches.</p>
        </div>

        <div className="about-section">
          <h2>Community</h2>
          <p>The best way to get involved, get an account set up, and get the latest VR client builds is to join the Team Beef Discord community: <a href="https://discord.gg/tuDB2YNc7h">discord.gg/tuDB2YNc7h</a></p>
        </div>
      </div>

      {showUserManagement && auth.isAdmin && auth.token && (
        <UserManagement
          token={auth.token}
          currentUsername={auth.username!}
          onClose={() => setShowUserManagement(false)}
        />
      )}
    </div>
  )
}
