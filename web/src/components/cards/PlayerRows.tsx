// Player table for team / FFA cards. Live shows ping; finished shows
// F+D+Score. Awards collapse into one icon per type with a count
// badge. CTF/1FCTF carrier renders to the right of the name.
import { ColoredText } from '../ColoredText'
import { PlayerPortrait } from '../PlayerPortrait'
import { PlayerBadge } from '../PlayerBadge'
import { BotBadge } from '../BotBadge'
import { MedalIcon } from '../MedalIcon'
import { FlagIcon } from '../FlagIcon'
import type { AwardEntry } from './format'

export interface PlayerRowData {
  name: string
  cleanName?: string
  /** Q3 model id used by PlayerPortrait. */
  model?: string
  team?: 1 | 2 | 3 | undefined  // 1=red, 2=blue, 3=spec, undef=free
  isBot?: boolean
  /** Bot skill 1–5; drives BotBadge color. Ignored for humans. */
  skill?: number
  isVR?: boolean
  isVerified?: boolean
  isAdmin?: boolean
  /** "score" for live; undefined for finished (use frags instead) */
  score?: number
  frags?: number
  deaths?: number
  ping?: number
  awards?: AwardEntry[]
  /** DB player ID — opens PlayerStatsModal on click. */
  playerId?: number
  /** Active CTF / 1FCTF flag this player is carrying, if any. */
  flagCarrier?: 'red' | 'blue' | 'neutral'
  /** Finished matches: false ⇒ dropped before the match ended,
   *  rendered with a gray team-dot regardless of team. */
  completed?: boolean
}

type Mode = 'live' | 'finished'

interface PlayerRowsProps {
  players: PlayerRowData[]
  mode: Mode
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

export function PlayerRows({ players, mode, onPlayerClick }: PlayerRowsProps) {
  return (
    <div className="player-rows">
      <div className={`player-row header ${mode}`}>
        <span>Player</span>
        <span style={{ textAlign: 'right' }}>{mode === 'live' ? 'Score' : 'F'}</span>
        {mode === 'finished' && <span style={{ textAlign: 'right' }}>D</span>}
        {mode === 'finished' && <span style={{ textAlign: 'right' }}>Score</span>}
        {mode === 'live' && <span style={{ textAlign: 'right' }}>Ping</span>}
      </div>
      {players.map((p, i) => {
        const portraitFallback = (p.cleanName || p.name || '?').replace(/\^./g, '').charAt(0).toLowerCase() || '?'
        const clickable = !!onPlayerClick && !p.isBot
        const handleClick = clickable
          ? () => onPlayerClick!(p.name, p.cleanName ?? p.name, p.playerId)
          : undefined
        return (
          <div
            key={i}
            className={`player-row ${mode} ${clickable ? 'clickable' : ''}`}
            onClick={handleClick}
            role={clickable ? 'button' : undefined}
            tabIndex={clickable ? 0 : undefined}
            onKeyDown={clickable ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                handleClick!()
              }
            } : undefined}
          >
            <span className="pname-cell">
              <span
                className={`team-dot ${p.completed === false ? 'dropped' : teamClass(p.team)}`}
                aria-hidden
                title={p.completed === false ? "Didn't finish the match" : undefined}
              />
              <span className="player-row__portrait">
                <PlayerPortrait model={p.model} size="sm" fallback={portraitFallback} />
              </span>
              {p.isBot ? (
                <BotBadge isBot skill={p.skill ?? 1} size="sm" />
              ) : (
                <PlayerBadge isVerified={p.isVerified} isAdmin={p.isAdmin} isVR={p.isVR} size="sm" />
              )}
              <span className={`name ${p.isBot ? 'bot' : ''}`}><ColoredText text={p.name} /></span>
              {p.flagCarrier && (
                <span className="row-carrier" aria-hidden>
                  {/* Static flag silhouette next to the name — the
                      runner icon (status="taken") is reserved for
                      score-cell indicators. */}
                  <FlagIcon team={p.flagCarrier} status="base" size="sm" />
                </span>
              )}
              {p.awards && p.awards.length > 0 && (
                <span className="row-awards">
                  {p.awards.map((a) => (
                    <MedalIcon key={a.type} type={a.type} count={a.count} size="sm" />
                  ))}
                </span>
              )}
            </span>
            <span className="stat">
              {mode === 'live' ? p.score ?? 0 : p.frags ?? 0}
            </span>
            {mode === 'finished' && <span className="stat dim">{p.deaths ?? 0}</span>}
            {mode === 'finished' && <span className="stat">{p.score ?? p.frags ?? 0}</span>}
            {mode === 'live' && <span className="stat dim">{p.ping ?? 0}</span>}
          </div>
        )
      })}
    </div>
  )
}

function teamClass(t?: number) {
  if (t === 1) return 'red'
  if (t === 2) return 'blue'
  if (t === 3) return 'spec'
  // Free / undefined team — covers FFA, where the player has no team
  // affiliation but was actively in the match. Distinct from .dropped.
  return 'free'
}
