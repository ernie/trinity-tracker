import { useState, useEffect, useRef, FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AppLogo } from './AppLogo'
import { PageNav } from './PageNav'
import { ColoredText } from './ColoredText'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerRecentMatches } from './PlayerRecentMatches'
import { UserManagement } from './UserManagement'
import { StatItem } from './StatItem'
import { PeriodSelector } from './PeriodSelector'
import { useAuth } from '../hooks/useAuth'
import { usePlayerStats } from '../hooks/usePlayerStats'
import { formatDate, formatDateTime, formatDuration } from '../utils/formatters'
import type { AccountProfile, TimePeriod } from '../types'

export function AccountPage() {
  const navigate = useNavigate()
  const { auth, loading: authLoading, logout, changePassword } = useAuth()

  const [profile, setProfile] = useState<AccountProfile | null>(null)
  const [period, setPeriod] = useState<TimePeriod>('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const { stats } = usePlayerStats(profile?.player?.id, period)

  // Link code state
  const [linkCode, setLinkCode] = useState<string | null>(null)
  const [expiresAt, setExpiresAt] = useState<Date | null>(null)
  const [timeRemaining, setTimeRemaining] = useState(0)
  const [generatingCode, setGeneratingCode] = useState(false)
  const [linkError, setLinkError] = useState('')
  const timerRef = useRef<number | null>(null)

  // Password change state
  const [showPasswordForm, setShowPasswordForm] = useState(false)
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [changingPassword, setChangingPassword] = useState(false)
  const [passwordSuccess, setPasswordSuccess] = useState(false)
  const [showUserManagement, setShowUserManagement] = useState(false)

  // Redirect if not authenticated (after auth check completes)
  useEffect(() => {
    if (!authLoading && !auth.isAuthenticated) {
      navigate('/')
    }
  }, [authLoading, auth.isAuthenticated, navigate])

  // Fetch profile on mount
  useEffect(() => {
    if (!auth.token) return

    setLoading(true)
    fetch('/api/account/profile', {
      headers: {
        Authorization: `Bearer ${auth.token}`,
      },
    })
      .then((res) => {
        if (!res.ok) throw new Error('Failed to load profile')
        return res.json()
      })
      .then((data) => setProfile(data))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false))
  }, [auth.token])

  // Link code countdown timer
  useEffect(() => {
    if (!expiresAt) return

    const updateTimer = () => {
      const now = new Date()
      const remaining = Math.max(0, Math.floor((expiresAt.getTime() - now.getTime()) / 1000))
      setTimeRemaining(remaining)

      if (remaining === 0) {
        setLinkCode(null)
        setExpiresAt(null)
      }
    }

    updateTimer()
    timerRef.current = window.setInterval(updateTimer, 1000)

    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current)
      }
    }
  }, [expiresAt])

  const generateCode = async () => {
    if (!auth.token) return

    setGeneratingCode(true)
    setLinkError('')

    try {
      const res = await fetch('/api/account/link-code', {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${auth.token}`,
        },
      })

      if (!res.ok) {
        const data = await res.json()
        setLinkError(data.error || 'Failed to generate code')
        return
      }

      const data = await res.json()
      setLinkCode(data.code)
      setExpiresAt(new Date(data.expires_at))
    } catch {
      setLinkError('Network error')
    } finally {
      setGeneratingCode(false)
    }
  }

  const formatTime = (seconds: number): string => {
    const mins = Math.floor(seconds / 60)
    const secs = seconds % 60
    return `${mins}:${secs.toString().padStart(2, '0')}`
  }

  const handlePasswordSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setPasswordError('')
    setPasswordSuccess(false)

    if (newPassword.length < 8) {
      setPasswordError('Password must be at least 8 characters')
      return
    }

    if (newPassword !== confirmPassword) {
      setPasswordError('Passwords do not match')
      return
    }

    setChangingPassword(true)
    const result = await changePassword(currentPassword, newPassword)
    setChangingPassword(false)

    if (!result.success) {
      setPasswordError(result.error || 'Failed to change password')
    } else {
      setPasswordSuccess(true)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setShowPasswordForm(false)
    }
  }

  if (authLoading || !auth.isAuthenticated) {
    return (
      <div className="account-page">
        <div className="loading">Loading...</div>
      </div>
    )
  }

  return (
    <div className="account-page">
      <header className="account-header">
        <h1>
          <AppLogo />
          My Account
        </h1>
        <PageNav />
        <div className="auth-section">
          <div className="user-info">
            <Link to="/account" className="username-link">{auth.username}</Link>
            {auth.isAdmin && (
              <button onClick={() => setShowUserManagement(true)} className="admin-btn">Users</button>
            )}
            <button onClick={logout} className="logout-btn">Logout</button>
          </div>
        </div>
      </header>

      {loading && <div className="loading">Loading...</div>}
      {error && <div className="error-message">{error}</div>}

      {profile && (
        <div className="account-columns">
          {/* Left Column: Player Profile & GUIDs */}
          <div className="account-left">
            {/* Player Profile with Stats */}
            <section className="account-section">
              <h2>Player Profile</h2>
              {profile.player ? (
                <div className="player-profile-inline">
                  <div className="player-name-row">
                    <PlayerPortrait model={profile.player.model} size="xl" />
                    <div className="player-name-info">
                      <div className="player-name-large">
                        <ColoredText text={profile.player.name} />
                      </div>
                      <div className="player-dates">
                        <div>Playing since {formatDate(profile.player.first_seen)}</div>
                        {profile.player.total_playtime_seconds > 0 && (
                          <div>{formatDuration(profile.player.total_playtime_seconds)} played</div>
                        )}
                      </div>
                    </div>
                  </div>

                  {/* Period selector */}
                  <PeriodSelector period={period} onChange={setPeriod} />

                  {/* Stats grid */}
                  {stats && (
                    <div className="stats-grid">
                      <StatItem
                        label="Matches"
                        value={stats.stats.completed_matches}
                        subscript={stats.stats.uncompleted_matches > 0 ? stats.stats.uncompleted_matches : undefined}
                        title={stats.stats.uncompleted_matches > 0
                          ? `${stats.stats.completed_matches} completed, ${stats.stats.uncompleted_matches} incomplete`
                          : undefined}
                      />
                      <StatItem label="K/D" value={stats.stats.kd_ratio.toFixed(2)} />
                      <StatItem label="Kills" value={stats.stats.kills} className="kills" />
                      <StatItem label="Deaths" value={stats.stats.deaths} className="deaths" />
                      <StatItem label="Victories" value={stats.stats.victories} />
                      <StatItem label="Excellent" value={stats.stats.excellents} />
                      <StatItem label="Impressive" value={stats.stats.impressives} />
                      <StatItem label="Humiliation" value={stats.stats.humiliations} />
                      <StatItem label="Captures" value={stats.stats.captures} />
                      <StatItem label="Returns" value={stats.stats.flag_returns} />
                      <StatItem label="Assists" value={stats.stats.assists} />
                      <StatItem label="Defense" value={stats.stats.defends} />
                    </div>
                  )}
                </div>
              ) : (
                <p className="no-player">No player profile linked to this account.</p>
              )}
            </section>

            {/* GUIDs Section */}
            {profile.guids && profile.guids.length > 0 && (
              <section className="account-section">
                <h2>Your Game Identities ({profile.guids.length})</h2>
                <div className="guids-list">
                  {profile.guids.map((guid) => (
                    <div key={guid.id} className="guid-item">
                      <div className="guid-main">
                        <ColoredText text={guid.name} />
                        <span className="guid-hash" title={guid.guid}>{guid.guid.slice(0, 8)}...</span>
                      </div>
                      <div className="guid-dates">
                        {formatDate(guid.first_seen)} - {formatDate(guid.last_seen)}
                      </div>
                    </div>
                  ))}
                </div>
              </section>
            )}

            {/* Recent Matches Section */}
            {profile.player && (
              <section className="account-section">
                <PlayerRecentMatches playerId={profile.player.id} />
              </section>
            )}
          </div>

          {/* Right Column: Account Info, Password, Link */}
          <div className="account-right">
            {/* Account Information */}
            <section className="account-section">
              <h2>Account Information</h2>
              <dl className="info-list">
                <dt>Username</dt>
                <dd>{profile.user.username}</dd>
                <dt>Role</dt>
                <dd>{profile.user.is_admin ? 'Administrator' : 'User'}</dd>
                <dt>Account Created</dt>
                <dd>{formatDateTime(profile.user.created_at)}</dd>
                <dt>Last Login</dt>
                <dd>{profile.user.last_login ? formatDateTime(profile.user.last_login) : 'First login'}</dd>
              </dl>
            </section>

            {/* Change Password */}
            <section className="account-section">
              <h2>Change Password</h2>
              {passwordSuccess && (
                <div className="success-message">Password changed successfully!</div>
              )}
              {showPasswordForm ? (
                <form onSubmit={handlePasswordSubmit} className="password-form">
                  <div className="form-group">
                    <label>Current Password</label>
                    <input
                      type="password"
                      value={currentPassword}
                      onChange={(e) => setCurrentPassword(e.target.value)}
                      disabled={changingPassword}
                      autoComplete="current-password"
                    />
                  </div>
                  <div className="form-group">
                    <label>New Password</label>
                    <input
                      type="password"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      disabled={changingPassword}
                      autoComplete="new-password"
                    />
                  </div>
                  <div className="form-group">
                    <label>Confirm New Password</label>
                    <input
                      type="password"
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      disabled={changingPassword}
                      autoComplete="new-password"
                    />
                  </div>
                  {passwordError && <div className="error-message">{passwordError}</div>}
                  <div className="form-actions">
                    <button
                      type="submit"
                      disabled={changingPassword || !currentPassword || !newPassword || !confirmPassword}
                    >
                      {changingPassword ? 'Changing...' : 'Change Password'}
                    </button>
                    <button
                      type="button"
                      className="cancel-btn"
                      onClick={() => {
                        setShowPasswordForm(false)
                        setCurrentPassword('')
                        setNewPassword('')
                        setConfirmPassword('')
                        setPasswordError('')
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              ) : (
                <button onClick={() => setShowPasswordForm(true)} className="change-password-btn">
                  Change Password
                </button>
              )}
            </section>

            {/* Link Additional GUID */}
            {profile.player && (
              <section className="account-section">
                <h2>Link Additional GUID</h2>
                <div className="link-explanation">
                  <p>
                    If you play on multiple computers or reinstall Quake 3, you may get a different
                    player ID (GUID) and see a separate profile for yourself.
                  </p>
                  <p>
                    <strong>To minimize extra GUIDs: copy your <code>qkey</code> file between ioquake3 installations,
                    and set <code>cl_guidServerUniq 0</code> in your config.</strong>
                  </p>
                </div>

                {linkError && <div className="error-message">{linkError}</div>}

                {linkCode ? (
                  <div className="code-display">
                    <div className="code-label">Your Link Code:</div>
                    <div className="code-value">{linkCode}</div>
                    <div className="code-timer">
                      Expires in: <strong>{formatTime(timeRemaining)}</strong>
                    </div>
                    <div className="code-instruction">
                      In game, type: <code>!link {linkCode}</code>
                    </div>
                    <button onClick={generateCode} disabled={generatingCode} className="generate-btn">
                      {generatingCode ? 'Generating...' : 'Generate New Code'}
                    </button>
                  </div>
                ) : (
                  <button className="generate-btn" onClick={generateCode} disabled={generatingCode}>
                    {generatingCode ? 'Generating...' : 'Generate Link Code'}
                  </button>
                )}
              </section>
            )}
          </div>
        </div>
      )}

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
