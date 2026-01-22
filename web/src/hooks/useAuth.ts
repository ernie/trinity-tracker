import { useState, useEffect, useCallback } from 'react'
import type { AuthState, LoginCredentials } from '../types'

const TOKEN_KEY = 'q3a_auth_token'

export function useAuth() {
  const [auth, setAuth] = useState<AuthState>({
    isAuthenticated: false,
    username: null,
    token: null,
    isAdmin: false,
    playerId: null,
    passwordChangeRequired: false,
  })
  const [loading, setLoading] = useState(true)

  // Check existing token on mount
  useEffect(() => {
    const token = sessionStorage.getItem(TOKEN_KEY)
    if (token) {
      checkToken(token)
    } else {
      setLoading(false)
    }
  }, [])

  const checkToken = async (token: string) => {
    try {
      const res = await fetch('/api/auth/check', {
        headers: { Authorization: `Bearer ${token}` },
      })
      const data = await res.json()
      if (data.authenticated) {
        setAuth({
          isAuthenticated: true,
          username: data.username,
          token,
          isAdmin: data.is_admin || false,
          playerId: data.player_id || null,
          passwordChangeRequired: data.password_change_required || false,
        })
      } else {
        sessionStorage.removeItem(TOKEN_KEY)
      }
    } catch {
      sessionStorage.removeItem(TOKEN_KEY)
    } finally {
      setLoading(false)
    }
  }

  const login = useCallback(async (credentials: LoginCredentials): Promise<boolean> => {
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(credentials),
      })

      if (!res.ok) return false

      const data = await res.json()
      sessionStorage.setItem(TOKEN_KEY, data.token)
      setAuth({
        isAuthenticated: true,
        username: data.username,
        token: data.token,
        isAdmin: data.is_admin || false,
        playerId: data.player_id || null,
        passwordChangeRequired: data.password_change_required || false,
      })
      return true
    } catch {
      return false
    }
  }, [])

  const logout = useCallback(() => {
    sessionStorage.removeItem(TOKEN_KEY)
    setAuth({
      isAuthenticated: false,
      username: null,
      token: null,
      isAdmin: false,
      playerId: null,
      passwordChangeRequired: false,
    })
  }, [])

  const changePassword = useCallback(async (currentPassword: string, newPassword: string): Promise<{ success: boolean; error?: string }> => {
    try {
      const res = await fetch('/api/auth/change-password', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${auth.token}`,
        },
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword,
        }),
      })

      const data = await res.json()
      if (!res.ok) {
        return { success: false, error: data.error || 'Failed to change password' }
      }

      // Update token after password change
      if (data.token) {
        sessionStorage.setItem(TOKEN_KEY, data.token)
        setAuth(prev => ({
          ...prev,
          token: data.token,
          passwordChangeRequired: false,
        }))
      }
      return { success: true }
    } catch {
      return { success: false, error: 'Network error' }
    }
  }, [auth.token])

  return { auth, loading, login, logout, changePassword }
}
