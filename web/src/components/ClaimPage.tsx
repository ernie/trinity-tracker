import { useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { ColoredText } from './ColoredText'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerBadge } from './PlayerBadge'
import { StatItem } from './StatItem'
import { Header } from './Header'
import { useAuth } from '../hooks/useAuth'
import { formatDate, formatDuration } from '../utils/formatters'
import type { PlayerProfile, AggregatedStats } from '../types'

interface ClaimInfo {
  code_id: number
  player_id: number
  player: PlayerProfile
  stats?: AggregatedStats
}

type ClaimStep = 'code_entry' | 'validated' | 'register' | 'login' | 'success'

function ClaimPlayerCard({ player, stats }: { player: PlayerProfile; stats?: AggregatedStats }) {
  return (
    <div className="claim-player-card">
      <div className="player-name-row">
        <PlayerPortrait model={player.model} size="xl" />
        <div className="player-name-info">
          <div className="player-name-large">
            <PlayerBadge isVerified={player.is_verified} isAdmin={player.is_admin} isVR={player.is_vr} size="md" />
            <ColoredText text={player.name} />
          </div>
          <div className="player-dates">
            <div>Playing since {formatDate(player.first_seen)}</div>
            {player.total_playtime_seconds > 0 && (
              <div>{formatDuration(player.total_playtime_seconds)} played</div>
            )}
          </div>
        </div>
      </div>

      {stats && stats.completed_matches > 0 && (
        <div className="stats-grid">
          <StatItem label="Matches" value={stats.completed_matches} />
          <StatItem label="K/D" value={stats.kd_ratio.toFixed(2)} />
          <StatItem label="Frags" value={stats.frags} className="frags" />
          <StatItem label="Deaths" value={stats.deaths} className="deaths" />
          <StatItem label="Victories" value={stats.victories} backgroundIcon="/assets/medals/medal_victory.png" />
          <StatItem label="Excellent" value={stats.excellents} backgroundIcon="/assets/medals/medal_excellent.png" />
          <StatItem label="Impressive" value={stats.impressives} backgroundIcon="/assets/medals/medal_impressive.png" />
          <StatItem label="Humiliation" value={stats.humiliations} backgroundIcon="/assets/medals/medal_gauntlet.png" />
          <StatItem label="Captures" value={stats.captures} backgroundIcon="/assets/medals/medal_capture.png" />
          <StatItem label="Returns" value={stats.flag_returns} backgroundIcon="/assets/flags/flag_in_base_red.png" />
          <StatItem label="Assists" value={stats.assists} backgroundIcon="/assets/medals/medal_assist.png" />
          <StatItem label="Defense" value={stats.defends} backgroundIcon="/assets/medals/medal_defend.png" />
        </div>
      )}
    </div>
  )
}

export function ClaimPage() {
  const navigate = useNavigate()
  const { auth, login } = useAuth()

  const [step, setStep] = useState<ClaimStep>('code_entry')
  const [code, setCode] = useState('')
  const [claimInfo, setClaimInfo] = useState<ClaimInfo | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  // Register form
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  // Login form
  const [loginUsername, setLoginUsername] = useState('')
  const [loginPassword, setLoginPassword] = useState('')

  const [successMessage, setSuccessMessage] = useState('')

  const handleValidate = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const res = await fetch('/api/claim/validate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code }),
      })

      const data = await res.json()
      if (!res.ok) {
        setError(data.error || 'Invalid or expired claim code')
        return
      }

      setClaimInfo(data)
      setStep('validated')
    } catch {
      setError('Network error')
    } finally {
      setLoading(false)
    }
  }

  const handleRegister = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (password.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }

    if (password !== confirmPassword) {
      setError('Passwords do not match')
      return
    }

    setLoading(true)

    try {
      const res = await fetch('/api/claim/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          code,
          username,
          password,
          confirm_password: confirmPassword,
        }),
      })

      const data = await res.json()
      if (!res.ok) {
        setError(data.error || 'Failed to create account')
        return
      }

      sessionStorage.setItem('q3a_auth_token', data.token)
      setSuccessMessage(`Account "${username}" created and player linked!`)
      setStep('success')
    } catch {
      setError('Network error')
    } finally {
      setLoading(false)
    }
  }

  const handleLogin = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const success = await login({ username: loginUsername, password: loginPassword })
      if (!success) {
        setError('Invalid credentials')
        setLoading(false)
        return
      }

      const token = sessionStorage.getItem('q3a_auth_token')
      const res = await fetch('/api/claim/link', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ code }),
      })

      const data = await res.json()
      if (!res.ok) {
        setError(data.error || 'Failed to link player')
        return
      }

      setSuccessMessage('Player linked to your account!')
      setStep('success')
    } catch {
      setError('Network error')
    } finally {
      setLoading(false)
    }
  }

  const handleMerge = async () => {
    setError('')
    setLoading(true)

    try {
      const res = await fetch('/api/claim/link', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${auth.token}`,
        },
        body: JSON.stringify({ code }),
      })

      const data = await res.json()
      if (!res.ok) {
        setError(data.error || 'Failed to link player')
        return
      }

      setSuccessMessage('Player linked to your account!')
      setStep('success')
    } catch {
      setError('Network error')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="claim-page">
      <Header title="Claim Identity" className="claim-header" />

      <div className="claim-container">
        {step === 'code_entry' && (
          <section className="claim-section">
            <p className="claim-instructions">
              Enter the 6-digit code from the <code>!claim</code> command in-game.
            </p>
            <form onSubmit={handleValidate} className="claim-form">
              <div className="claim-code-input-group">
                <input
                  type="text"
                  value={code}
                  onChange={(e) => {
                    const val = e.target.value.replace(/\D/g, '').slice(0, 6)
                    setCode(val)
                    setError('')
                  }}
                  placeholder="000000"
                  maxLength={6}
                  className="claim-code-input"
                  autoFocus
                />
                <button
                  type="submit"
                  disabled={code.length !== 6 || loading}
                  className="claim-submit-btn"
                >
                  {loading ? 'Validating...' : 'Submit'}
                </button>
              </div>
              {error && <div className="claim-error centered">{error}</div>}
            </form>
          </section>
        )}

        {step === 'validated' && claimInfo && (
          <section className="claim-section">
            <ClaimPlayerCard player={claimInfo.player} stats={claimInfo.stats} />

            {auth.isAuthenticated ? (
              <div className="claim-merge-section">
                <p>Link this player to your account (<strong>{auth.username}</strong>)?</p>
                <div className="claim-actions">
                  <button
                    onClick={handleMerge}
                    disabled={loading}
                    className="claim-primary-btn"
                  >
                    {loading ? 'Linking...' : 'Link Player'}
                  </button>
                  <button
                    onClick={() => { setStep('code_entry'); setError(''); setClaimInfo(null); setCode(''); }}
                    className="claim-secondary-btn"
                  >
                    Cancel
                  </button>
                </div>
                {error && <div className="claim-error">{error}</div>}
              </div>
            ) : (
              <div className="claim-choice-section">
                <div className="claim-actions">
                  <button
                    onClick={() => { setStep('register'); setError(''); }}
                    className="claim-primary-btn"
                  >
                    Create Account
                  </button>
                  <button
                    onClick={() => { setStep('login'); setError(''); }}
                    className="claim-secondary-btn"
                  >
                    I already have an account
                  </button>
                </div>
              </div>
            )}
          </section>
        )}

        {step === 'register' && claimInfo && (
          <section className="claim-section">
            <ClaimPlayerCard player={claimInfo.player} stats={claimInfo.stats} />

            <h3>Create Account</h3>
            <form onSubmit={handleRegister} className="claim-form vertical">
              <div className="claim-form-group">
                <label htmlFor="username">Username</label>
                <input
                  id="username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  autoFocus
                />
              </div>
              <div className="claim-form-group">
                <label htmlFor="password">Password</label>
                <input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  minLength={8}
                />
              </div>
              <div className="claim-form-group">
                <label htmlFor="confirm-password">Confirm Password</label>
                <input
                  id="confirm-password"
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                />
              </div>
              {error && <div className="claim-error">{error}</div>}
              <div className="claim-actions">
                <button type="submit" disabled={loading} className="claim-primary-btn">
                  {loading ? 'Creating...' : 'Create Account'}
                </button>
                <button
                  type="button"
                  onClick={() => { setStep('validated'); setError(''); }}
                  className="claim-secondary-btn"
                >
                  Back
                </button>
              </div>
            </form>
          </section>
        )}

        {step === 'login' && claimInfo && (
          <section className="claim-section">
            <ClaimPlayerCard player={claimInfo.player} stats={claimInfo.stats} />

            <h3>Log In to Link</h3>
            <form onSubmit={handleLogin} className="claim-form vertical">
              <div className="claim-form-group">
                <label htmlFor="login-username">Username</label>
                <input
                  id="login-username"
                  type="text"
                  value={loginUsername}
                  onChange={(e) => setLoginUsername(e.target.value)}
                  required
                  autoFocus
                />
              </div>
              <div className="claim-form-group">
                <label htmlFor="login-password">Password</label>
                <input
                  id="login-password"
                  type="password"
                  value={loginPassword}
                  onChange={(e) => setLoginPassword(e.target.value)}
                  required
                />
              </div>
              {error && <div className="claim-error">{error}</div>}
              <div className="claim-actions">
                <button type="submit" disabled={loading} className="claim-primary-btn">
                  {loading ? 'Logging in...' : 'Log In & Link'}
                </button>
                <button
                  type="button"
                  onClick={() => { setStep('validated'); setError(''); }}
                  className="claim-secondary-btn"
                >
                  Back
                </button>
              </div>
            </form>
          </section>
        )}

        {step === 'success' && (
          <section className="claim-section claim-success">
            <div className="claim-success-icon">&#10003;</div>
            <p>{successMessage}</p>
            <button onClick={() => navigate('/account')} className="claim-primary-btn">
              Go to Account
            </button>
          </section>
        )}
      </div>
    </div>
  )
}
