import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { BotBadge } from './BotBadge'
import { ColoredText } from './ColoredText'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerBadge } from './PlayerBadge'
import { StatItem } from './StatItem'
import { PeriodSelector } from './PeriodSelector'
import { usePlayerStats } from '../hooks/usePlayerStats'
import { formatDate, formatDuration } from '../utils/formatters'
import type { TimePeriod, PlayerStatsResponse, PlayerName } from '../types'

interface PlayerStatsModalProps {
  playerName: string
  playerId: number
  onClose: () => void
}

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
      <div className="player-stats-modal">
        <div className="modal-header">
          <h3>
            {stats && <PlayerPortrait model={stats.player.model} size="md" />}
            {stats?.player.is_bot && <BotBadge isBot skill={5} size="md" />}
            {stats && !stats.player.is_bot && <PlayerBadge playerId={stats.player.id} isVR={stats.player.is_vr} size="md" />}
            <ColoredText text={playerName} />
          </h3>
          <button onClick={onClose} className="close-btn" aria-label="Close">×</button>
        </div>

        <PeriodSelector period={period} onChange={setPeriod} />

        <div className="modal-content">
          {loading ? (
            <div className="stats-loading">Loading stats...</div>
          ) : error ? (
            <div className="stats-error">{error}</div>
          ) : stats ? (
            <StatsDisplay stats={stats} />
          ) : null}
        </div>
      </div>
    </div>
  )
}

function StatsDisplay({ stats }: { stats: PlayerStatsResponse }) {
  return (
    <>
      <div className="player-meta-top">
        <span><em>Seen:</em> {formatDate(stats.player.first_seen)} – {formatDate(stats.player.last_seen)}</span>
        {stats.player.total_playtime_seconds > 0 && (
          <span><em>Played:</em> {formatDuration(stats.player.total_playtime_seconds)}</span>
        )}
      </div>

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
        <StatItem label="Frags" value={stats.stats.frags} className="frags" />
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

      {stats.names && (() => {
        const uniqueNames = [...new Set(stats.names.map((n: PlayerName) => n.name))].filter(name => name !== stats.player.name)
        return uniqueNames.length > 0 && (
          <div className="also-known-as">
            <h4>Also known as</h4>
            <div className="name-list">
              {uniqueNames.slice(0, 5).map((name: string, i: number) => (
                <span key={i} className="aka-name">
                  <ColoredText text={name} />
                </span>
              ))}
            </div>
          </div>
        )
      })()}

      <div className="modal-footer">
        <Link to={`/players/${stats.player.id}`} className="view-profile-link">
          View full profile
        </Link>
      </div>
    </>
  )
}
