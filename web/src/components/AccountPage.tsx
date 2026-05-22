import { useState, useEffect, useRef, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { ColoredText } from './ColoredText'
import { PlayerHero } from './PlayerHero'
import { HonorsPanel } from './HonorsPanel'
import { PlayerRecentMatches } from './PlayerRecentMatches'
import { PeriodSelector } from './PeriodSelector'
import { useAuth } from '../hooks/useAuth'
import { usePlayerStats } from '../hooks/usePlayerStats'
import { formatDate, formatDateTime } from '../utils/formatters'
import type { AccountProfile, TimePeriod } from '../types'

export function AccountPage() {
  const navigate = useNavigate()
  const { auth, loading: authLoading, changePassword } = useAuth()

  const [profile, setProfile] = useState<AccountProfile | null>(null)
  const [period, setPeriod] = useState<TimePeriod>('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const { stats, setFeaturedHonor } = usePlayerStats(profile?.player?.id, period)

  // Link code state
  const [linkCode, setLinkCode] = useState<string | null>(null)
  const [expiresAt, setExpiresAt] = useState<Date | null>(null)
  const [timeRemaining, setTimeRemaining] = useState(0)
  const [generatingCode, setGeneratingCode] = useState(false)
  const [linkError, setLinkError] = useState('')
  const timerRef = useRef<number | null>(null)

  // Claim code state (for linking a claim code on this page)
  const [claimCode, setClaimCode] = useState('')
  const [claimLoading, setClaimLoading] = useState(false)
  const [claimError, setClaimError] = useState('')
  const [claimSuccess, setClaimSuccess] = useState(false)

  // Game token state
  const [gameToken, setGameToken] = useState<string | null>(null)
  const [gameTokenLoading, setGameTokenLoading] = useState(false)
  const [gameTokenError, setGameTokenError] = useState('')
  const [gameTokenCopied, setGameTokenCopied] = useState(false)
  const [gameTokenVisible, setGameTokenVisible] = useState(false)
  const [showRotateConfirm, setShowRotateConfirm] = useState(false)

  // Password change state
  const [showPasswordForm, setShowPasswordForm] = useState(false)
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [changingPassword, setChangingPassword] = useState(false)
  const [passwordSuccess, setPasswordSuccess] = useState(false)

  // Redirect if not authenticated (after auth check completes)
  useEffect(() => {
    if (!authLoading && !auth.isAuthenticated) {
      navigate('/')
    }
  }, [authLoading, auth.isAuthenticated, navigate])

  // Fetch profile on mount
  useEffect(() => {
    if (!auth.token) return

    // eslint-disable-next-line react-hooks/set-state-in-effect
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

  // Fetch game token on mount
  useEffect(() => {
    if (!auth.token) return

    // eslint-disable-next-line react-hooks/set-state-in-effect
    setGameTokenLoading(true)
    fetch('/api/auth/game-token', {
      headers: {
        Authorization: `Bearer ${auth.token}`,
      },
    })
      .then((res) => {
        if (!res.ok) throw new Error('Failed to load game token')
        return res.json()
      })
      .then((data) => setGameToken(data.token))
      .catch((err) => setGameTokenError(err.message))
      .finally(() => setGameTokenLoading(false))
  }, [auth.token])

  const handleCopyToken = async () => {
    if (!gameToken) return
    try {
      await navigator.clipboard.writeText(gameToken)
      setGameTokenCopied(true)
      setTimeout(() => setGameTokenCopied(false), 2000)
    } catch {
      setGameTokenError('Failed to copy to clipboard')
    }
  }

  const handleRotateToken = async () => {
    if (!auth.token) return

    setGameTokenLoading(true)
    setGameTokenError('')
    setShowRotateConfirm(false)

    try {
      const res = await fetch('/api/auth/game-token', {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${auth.token}`,
        },
      })

      if (!res.ok) {
        const data = await res.json()
        setGameTokenError(data.error || 'Failed to rotate token')
        return
      }

      const data = await res.json()
      setGameToken(data.token)
    } catch {
      setGameTokenError('Network error')
    } finally {
      setGameTokenLoading(false)
    }
  }

  const handleClaimLink = async () => {
    if (!auth.token || claimCode.length !== 6) return
    setClaimLoading(true)
    setClaimError('')
    setClaimSuccess(false)

    try {
      // First validate the code
      const validateRes = await fetch('/api/claim/validate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code: claimCode }),
      })
      if (!validateRes.ok) {
        const data = await validateRes.json()
        setClaimError(data.error || 'Invalid or expired claim code')
        return
      }

      // Then link it
      const res = await fetch('/api/claim/link', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${auth.token}`,
        },
        body: JSON.stringify({ code: claimCode }),
      })
      const data = await res.json()
      if (!res.ok) {
        setClaimError(data.error || 'Failed to link player')
        return
      }
      setClaimSuccess(true)
      setClaimCode('')
    } catch {
      setClaimError('Network error')
    } finally {
      setClaimLoading(false)
    }
  }

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
      {loading && <div className="loading">Loading...</div>}
      {error && <div className="error-message">{error}</div>}

      {profile && (
        <div className="account-columns">
          {/* Left Column: Player Profile & GUIDs */}
          <div className="account-left">
            {/* Player Profile with Stats */}
            <section className="account-section account-profile">
              <h2>Player Profile</h2>
              {profile.player ? (
                <>
                  {/* Use the same triad (PlayerHero → PeriodSelector →
                      HonorsPanel) as PlayersPage and PlayerStatsModal so
                      this view stays in lockstep with the others. */}
                  {stats ? (
                    <>
                      <PlayerHero player={stats.player} stats={stats.stats} variant="page" />
                      <PeriodSelector period={period} onChange={setPeriod} />
                      <HonorsPanel
                        stats={stats.stats}
                        featuredKey={stats.player.featured_honor}
                        isOwner
                        onFeaturedChange={(key) => { if (auth.token) void setFeaturedHonor(key, auth.token) }}
                      />
                    </>
                  ) : (
                    <PeriodSelector period={period} onChange={setPeriod} />
                  )}
                </>
              ) : (
                <p className="no-player">No player profile linked to this account.</p>
              )}
            </section>

            {/* GUIDs Section */}
            {profile.guids && profile.guids.length > 0 && (
              <section className="account-section account-guids">
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
              <section className="account-section account-matches">
                <PlayerRecentMatches playerId={profile.player.id} />
              </section>
            )}
          </div>

          {/* Right Column: Account Info, Password, Link */}
          <div className="account-right">
            {/* Account Information */}
            <section className="account-section account-info">
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
            <section className="account-section account-password">
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

            {/* Game Token */}
            <section className="account-section account-game-token">
              <h2>Game Token</h2>
              <p className="link-explanation-text">
                Use this token with <code>cl_trinityToken</code> in your engine config, or log in from the game menu.
              </p>

              {gameTokenError && <div className="error-message">{gameTokenError}</div>}

              {gameTokenLoading && !gameToken ? (
                <div className="loading">Loading...</div>
              ) : gameToken ? (
                <div className="game-token-display">
                  <div className={`game-token-value${gameTokenVisible ? '' : ' hidden'}`}>{gameToken}</div>
                  <div className="game-token-actions">
                    <button
                      className="generate-btn rotate-btn"
                      onClick={() => setGameTokenVisible(!gameTokenVisible)}
                    >
                      {gameTokenVisible ? 'Hide' : 'Reveal'}
                    </button>
                    <button
                      className="generate-btn rotate-btn"
                      onClick={handleCopyToken}
                    >
                      {gameTokenCopied ? 'Copied!' : 'Copy'}
                    </button>
                    {showRotateConfirm ? (
                      <div className="rotate-confirm">
                        <span className="rotate-confirm-text">Invalidate current token?</span>
                        <div className="rotate-confirm-actions">
                          <button
                            className="generate-btn"
                            onClick={handleRotateToken}
                            disabled={gameTokenLoading}
                          >
                            {gameTokenLoading ? 'Rotating...' : 'Confirm'}
                          </button>
                          <button
                            className="cancel-btn"
                            onClick={() => setShowRotateConfirm(false)}
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    ) : (
                      <button
                        className="generate-btn"
                        onClick={() => setShowRotateConfirm(true)}
                      >
                        Rotate
                      </button>
                    )}
                  </div>
                </div>
              ) : null}
            </section>

            {/* Link Game Identity */}
            <section className="account-section account-link">
              <h2>Link Game Identity</h2>

              <div className="link-method">
                <h3>From in-game</h3>
                <p className="link-explanation-text">
                  Type <code>!claim</code> in-game to get a code, then enter it here.
                </p>
                <div className="claim-code-inline">
                  <input
                    type="text"
                    value={claimCode}
                    onChange={(e) => {
                      const val = e.target.value.replace(/\D/g, '').slice(0, 6)
                      setClaimCode(val)
                      setClaimError('')
                      setClaimSuccess(false)
                    }}
                    placeholder="000000"
                    maxLength={6}
                    className="claim-code-field"
                  />
                  <button
                    onClick={handleClaimLink}
                    disabled={claimCode.length !== 6 || claimLoading}
                    className="generate-btn"
                  >
                    {claimLoading ? 'Linking...' : 'Link'}
                  </button>
                </div>
                {claimError && <div className="error-message">{claimError}</div>}
                {claimSuccess && <div className="success-message">Identity linked! Refresh to see changes.</div>}
              </div>

              {profile.player && (
                <div className="link-method">
                  <h3>From the web</h3>
                  <p className="link-explanation-text">
                    Generate a code here, then type <code>!link &lt;code&gt;</code> in-game.
                  </p>

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
                </div>
              )}
            </section>
          </div>
        </div>
      )}

    </div>
  )
}
