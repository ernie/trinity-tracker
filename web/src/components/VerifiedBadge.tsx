import { useVerifiedPlayers } from '../hooks/useVerifiedPlayers'

interface VerifiedBadgeProps {
  playerId: number
}

export function VerifiedBadge({ playerId }: VerifiedBadgeProps) {
  const { isVerifiedById, isAdminById } = useVerifiedPlayers()

  if (!isVerifiedById(playerId)) {
    return null
  }

  const admin = isAdminById(playerId)

  return (
    <span
      className={`verified-badge ${admin ? 'admin' : 'user'}`}
      title={admin ? 'Verified Admin' : 'Verified User'}
    >
      {admin ? '\u2605' : '\u2713'}
    </span>
  )
}
