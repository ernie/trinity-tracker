// Player table for team / FFA cards. Live mode shows ping; finished
// mode shows F + D + Score. Awards accumulate inline next to the name
// with a left-edge mask-fade so older medals dissolve into the name
// when the row gets crowded.
import { ColoredText } from '../ColoredText'
import { PlayerPortrait } from '../PlayerPortrait'
import { PlayerBadge } from '../PlayerBadge'
import { BotBadge } from '../BotBadge'
import { MedalIcon } from '../MedalIcon'
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
}

type Mode = 'live' | 'finished'

export function PlayerRows({ players, mode }: { players: PlayerRowData[]; mode: Mode }) {
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
        return (
          <div key={i} className={`player-row ${mode}`}>
            <span className="pname-cell">
              <span className={`team-dot ${teamClass(p.team)}`} aria-hidden />
              <span className="player-row__portrait">
                <PlayerPortrait model={p.model} size="sm" fallback={portraitFallback} />
              </span>
              {p.isBot ? (
                <BotBadge isBot skill={p.skill ?? 1} size="sm" />
              ) : (
                <PlayerBadge isVerified={p.isVerified} isAdmin={p.isAdmin} isVR={p.isVR} size="sm" />
              )}
              <span className={`name ${p.isBot ? 'bot' : ''}`}><ColoredText text={p.name} /></span>
              {p.awards && p.awards.length > 0 && (
                <span className="row-awards">
                  {p.awards.flatMap((a) =>
                    Array.from({ length: a.count }, (_, j) => (
                      <MedalIcon key={`${a.type}-${j}`} type={a.type} size="sm" showCount={false} />
                    ))
                  )}
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
  return ''
}
