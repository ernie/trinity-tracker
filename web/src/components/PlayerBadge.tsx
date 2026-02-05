import { useVerifiedPlayers } from '../hooks/useVerifiedPlayers'

interface PlayerBadgeProps {
  playerId: number
  isVR?: boolean
  size?: 'sm' | 'md' | 'lg'
}

export function PlayerBadge({ playerId, isVR, size = 'sm' }: PlayerBadgeProps) {
  const { isVerifiedById, isAdminById } = useVerifiedPlayers()
  const verified = isVerifiedById(playerId)
  const admin = isAdminById(playerId)

  if (!verified && !isVR) return null

  const sizeClass = `player-badge-${size}`

  // VR only (not verified): just show VR icon
  if (isVR && !verified) {
    return (
      <span className={`player-badge ${sizeClass} vr-only`} title="Plays in VR">
        <img src="/assets/vr/vr.png" alt="VR" />
      </span>
    )
  }

  // Verified only (no VR)
  if (!isVR) {
    return (
      <span
        className={`player-badge ${sizeClass} ${admin ? 'admin' : 'user'}`}
        title={admin ? 'Verified Admin' : 'Verified User'}
      >
        {admin ? '\u2605' : '\u2713'}
      </span>
    )
  }

  // Both VR + verified: symbol on top of VR icon background
  return (
    <span
      className={`player-badge ${sizeClass} ${admin ? 'admin' : 'user'} vr`}
      title={admin ? 'Verified Admin (VR)' : 'Verified User (VR)'}
    >
      {admin ? '\u2605' : '\u2713'}
    </span>
  )
}
