// Player table for team / FFA cards. Live mode shows ping; finished
// mode shows F + D + Score. Awards accumulate inline next to the name
// with a left-edge mask-fade so older medals dissolve into the name
// when the row gets crowded.
import { ColoredText } from '../ColoredText'

export interface PlayerRowData {
  name: string
  team?: 1 | 2 | 3 | undefined  // 1=red, 2=blue, 3=spec, undef=free
  isBot?: boolean
  /** "score" for live; undefined for finished (use F instead) */
  score?: number
  frags?: number
  deaths?: number
  ping?: number
  awards?: string[]
}

type Mode = 'live' | 'finished'

function MedalIconStub({ kind }: { kind: string }) {
  // Mockup placeholder — production callers can replace with <MedalIcon>.
  return <span className={`medal medal-${kind}`}>★</span>
}

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
      {players.map((p, i) => (
        <div key={i} className={`player-row ${mode}`}>
          <span className="pname-cell">
            <span className={`team-dot ${teamClass(p.team)}`} aria-hidden />
            <span className={`name ${p.isBot ? 'bot' : ''}`}><ColoredText text={p.name} /></span>
            {p.awards && p.awards.length > 0 && (
              <span className="row-awards">
                {p.awards.map((m, j) => <MedalIconStub key={j} kind={m} />)}
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
      ))}
    </div>
  )
}

function teamClass(t?: number) {
  if (t === 1) return 'red'
  if (t === 2) return 'blue'
  if (t === 3) return 'spec'
  return ''
}
