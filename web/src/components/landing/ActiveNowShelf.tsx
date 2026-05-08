import { Link } from 'react-router-dom'
import { useLiveData } from '../../contexts/LiveDataContext'
import { ServerCard } from '../ServerCard'

export function ActiveNowShelf() {
  const live = useLiveData()
  const activeServers = Array.from(live.servers.values()).filter(
    (s) => s.online && s.human_count > 0
  )

  // Hide silently if no active matches qualify
  if (activeServers.length === 0) return null

  return (
    <section className="landing-section">
      <header className="landing-section__head">
        <h2 className="landing-section__title">
          <span className="landing-section__pulse" aria-hidden /> IN PROGRESS, RIGHT NOW.
        </h2>
        <Link to="/servers" className="landing-section__cta">All servers →</Link>
      </header>
      <div className="landing-shelf-h">
        <div className="landing-shelf-h__scroll">
          {activeServers.map((server) => (
            <div key={server.server_id} className="landing-shelf-h__slot">
              <ServerCard
                server={server}
                newPlayers={live.newPlayers}
                onPlayerClick={live.showPlayer}
                liveness={live.liveness.get(server.server_id)}
              />
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
