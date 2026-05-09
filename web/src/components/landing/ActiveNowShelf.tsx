import { Link } from 'react-router-dom'
import { useLiveData } from '../../contexts/LiveDataContext'
import { ServerCard } from '../ServerCard'
import { ArrowIcon } from '../ArrowIcon'

export function ActiveNowShelf() {
  const live = useLiveData()
  const activeServers = Array.from(live.servers.values()).filter(
    (s) => s.online && s.human_count > 0
  )
  const totalOnline = live.servers.size > 0
    ? Array.from(live.servers.values()).filter((s) => s.online).length
    : 0

  return (
    <section className="landing-section">
      <header className="landing-section__head">
        <h2 className="landing-section__title">
          <span className="landing-section__pulse" aria-hidden /> IN PROGRESS, RIGHT NOW.
        </h2>
        <Link to="/servers" className="landing-section__cta">All servers <ArrowIcon direction="right" /></Link>
      </header>
      {activeServers.length > 0 ? (
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
      ) : (
        <div className="landing-quiet">
          <span className="landing-quiet__dot" aria-hidden />
          <p className="landing-quiet__title">ALL QUIET</p>
          <p className="landing-quiet__sub">
            No live matches right now.
            {totalOnline > 0 ? ` ${totalOnline} server${totalOnline === 1 ? '' : 's'} online and waiting.` : ''}
          </p>
          <Link to="/servers" className="landing-quiet__cta">Browse servers <ArrowIcon direction="right" /></Link>
        </div>
      )}
    </section>
  )
}
