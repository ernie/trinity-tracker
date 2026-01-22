import { useState, FormEvent } from 'react'

interface LoginFormProps {
  onLogin: (username: string, password: string) => Promise<boolean>
}

export function LoginForm({ onLogin }: LoginFormProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [expanded, setExpanded] = useState(false)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    const success = await onLogin(username, password)

    if (!success) {
      setError('Invalid credentials')
    }
    setLoading(false)
  }

  if (!expanded) {
    return (
      <button className="login-toggle" onClick={() => setExpanded(true)}>
        Login
      </button>
    )
  }

  return (
    <form className="login-form" onSubmit={handleSubmit}>
      <input
        type="text"
        placeholder="Username"
        value={username}
        onChange={(e) => setUsername(e.target.value)}
        disabled={loading}
        autoComplete="username"
      />
      <input
        type="password"
        placeholder="Password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        disabled={loading}
        autoComplete="current-password"
      />
      <button type="submit" disabled={loading || !username || !password}>
        {loading ? '...' : 'Login'}
      </button>
      <button type="button" className="cancel-btn" onClick={() => setExpanded(false)}>
        Cancel
      </button>
      {error && <span className="login-error">{error}</span>}
    </form>
  )
}
