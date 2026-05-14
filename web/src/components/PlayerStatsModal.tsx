import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { PeriodSelector } from './PeriodSelector'
import { PlayerHero } from './PlayerHero'
import { HonorsPanel } from './HonorsPanel'
import { PlayerAkaList } from './PlayerAkaList'
import { ArrowIcon } from './ArrowIcon'
import { usePlayerStats } from '../hooks/usePlayerStats'
import { useLiveData } from '../contexts/LiveDataContext'
import type { TimePeriod, PlayerStatsResponse } from '../types'

interface PlayerStatsModalProps {
  playerName: string
  playerId: number
  onClose: () => void
}

// Quick-look profile modal. Shares hero / honors / aka chrome with
// the full /players/:id page; the page is the same shape at a larger
// scale, plus admin sections + recent matches.
export function PlayerStatsModal({ playerName, playerId, onClose }: PlayerStatsModalProps) {
  const [period, setPeriod] = useState<TimePeriod>('all')
  const { stats, loading, error } = usePlayerStats(playerId, period)

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  const handleBackdropClick = useCallback((e: React.MouseEvent) => {
    if (e.target === e.currentTarget) onClose()
  }, [onClose])

  return (
    <div className="modal-backdrop" onClick={handleBackdropClick}>
      <div
        className="player-stats-modal"
        role="dialog"
        aria-label={`${playerName} stats`}
      >
        <button
          onClick={onClose}
          className="player-stats-modal__close"
          aria-label="Close"
        >
          &times;
        </button>

        <div className="player-stats-modal__content">
          {loading ? (
            <div className="stats-loading">Loading stats…</div>
          ) : error ? (
            <div className="stats-error">{error}</div>
          ) : stats ? (
            <ModalBody
              stats={stats}
              fallbackName={playerName}
              period={period}
              onPeriodChange={setPeriod}
              onClose={onClose}
            />
          ) : null}
        </div>
      </div>
    </div>
  )
}

interface ModalBodyProps {
  stats: PlayerStatsResponse
  fallbackName: string
  period: TimePeriod
  onPeriodChange: (p: TimePeriod) => void
  onClose: () => void
}

function ModalBody({ stats, fallbackName, period, onPeriodChange, onClose }: ModalBodyProps) {
  const { notifyDrillIn } = useLiveData()

  return (
    <>
      <PlayerHero
        player={stats.player}
        stats={stats.stats}
        variant="modal"
        fallbackName={fallbackName}
      />

      <PeriodSelector period={period} onChange={onPeriodChange} />

      <HonorsPanel
        stats={stats.stats}
        featuredKey={stats.player.featured_honor}
      />

      <PlayerAkaList names={stats.names} primaryName={stats.player.name} max={6} />

      <footer className="player-stats-modal__footer">
        <Link
          to={`/players/${stats.player.id}`}
          className="view-profile-link"
          onClick={() => { notifyDrillIn(); onClose() }}
        >
          View full profile <ArrowIcon direction="right" />
        </Link>
      </footer>
    </>
  )
}
