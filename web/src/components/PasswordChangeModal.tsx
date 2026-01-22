import { useState, FormEvent } from 'react'

interface PasswordChangeModalProps {
  required: boolean
  onPasswordChange: (currentPassword: string, newPassword: string) => Promise<{ success: boolean; error?: string }>
  onClose?: () => void
}

export function PasswordChangeModal({ required, onPasswordChange, onClose }: PasswordChangeModalProps) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }

    if (newPassword !== confirmPassword) {
      setError('Passwords do not match')
      return
    }

    setLoading(true)
    const result = await onPasswordChange(currentPassword, newPassword)
    setLoading(false)

    if (!result.success) {
      setError(result.error || 'Failed to change password')
    } else if (onClose) {
      onClose()
    }
  }

  return (
    <div className="modal-overlay">
      <div className="modal password-change-modal">
        <div className="modal-header">
          <h2>{required ? 'Password Change Required' : 'Change Password'}</h2>
          {!required && onClose && (
            <button className="close-btn" onClick={onClose}>&times;</button>
          )}
        </div>
        {required && (
          <p className="password-change-notice">
            You must change your password before continuing.
          </p>
        )}
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label>Current Password</label>
            <input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              disabled={loading}
              autoComplete="current-password"
            />
          </div>
          <div className="form-group">
            <label>New Password</label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              disabled={loading}
              autoComplete="new-password"
            />
          </div>
          <div className="form-group">
            <label>Confirm New Password</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              disabled={loading}
              autoComplete="new-password"
            />
          </div>
          {error && <div className="error-message">{error}</div>}
          <div className="modal-actions">
            <button type="submit" disabled={loading || !currentPassword || !newPassword || !confirmPassword}>
              {loading ? 'Changing...' : 'Change Password'}
            </button>
            {!required && onClose && (
              <button type="button" className="cancel-btn" onClick={onClose}>
                Cancel
              </button>
            )}
          </div>
        </form>
      </div>
    </div>
  )
}
