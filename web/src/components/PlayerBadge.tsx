interface PlayerBadgeProps {
  isVerified?: boolean
  isAdmin?: boolean
  isVR?: boolean
  size?: 'sm' | 'md' | 'lg'
}

const StarIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
  </svg>
)

const CheckIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z" />
  </svg>
)

const QuestionIcon = () => (
  <svg viewBox="0 0 16 16" fill="currentColor">
    <path d="M8 1.5C5.8 1.5 4 3.1 4 5.2h2.2c0-1 .8-1.7 1.8-1.7s1.8.7 1.8 1.7c0 .8-.5 1.2-1.3 1.8-.9.7-1.5 1.5-1.5 2.8h2.2c0-.9.4-1.3 1.2-1.9.9-.7 1.6-1.5 1.6-2.7C12 3.1 10.2 1.5 8 1.5zM7 12.5h2V14.5H7z" />
  </svg>
)

export function PlayerBadge({ isVerified, isAdmin, isVR, size = 'sm' }: PlayerBadgeProps) {
  const sizeClass = `player-badge-${size}`

  // Unverified player: show placeholder badge
  if (!isVerified && !isVR) {
    return (
      <span
        className={`player-badge ${sizeClass} unverified`}
        title="Unverified"
      >
        <QuestionIcon />
      </span>
    )
  }

  // VR only (not verified): VR icon with dotted outline
  if (isVR && !isVerified) {
    return (
      <span className={`player-badge ${sizeClass} unverified vr`} title="Unverified (VR)">
        <QuestionIcon />
      </span>
    )
  }

  // Verified only (no VR)
  if (!isVR) {
    return (
      <span
        className={`player-badge ${sizeClass} ${isAdmin ? 'admin' : 'user'}`}
        title={isAdmin ? 'Verified Admin' : 'Verified User'}
      >
        {isAdmin ? <StarIcon /> : <CheckIcon />}
      </span>
    )
  }

  // Both VR + verified: symbol on top of VR icon background
  return (
    <span
      className={`player-badge ${sizeClass} ${isAdmin ? 'admin' : 'user'} vr`}
      title={isAdmin ? 'Verified Admin (VR)' : 'Verified User (VR)'}
    >
      {isAdmin ? <StarIcon /> : <CheckIcon />}
    </span>
  )
}
