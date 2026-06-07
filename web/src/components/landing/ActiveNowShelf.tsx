import { Link } from "react-router-dom";
import { useLiveData } from "../../contexts/LiveDataContext";
import { ServerCard } from "../ServerCard";
import { ArrowIcon } from "../ArrowIcon";
import { NavScroller } from "../NavScroller";

// Bot-spectating featured set, in display order. Five entries to match
// the Recent Matches shelf's SHELF_SIZE.
const FEATURED_BOT_KEYS = ["overload", "1fctf", "ctf-ta", "1v1", "ffa"];

export function ActiveNowShelf() {
  const live = useLiveData();
  const allServers = Array.from(live.servers.values());
  const activeServers = allServers.filter((s) => s.online && s.human_count > 0);
  // No humans anywhere: offer a few bot matches that are on air instead of
  // an empty shelf. Fixed hub picks across the distinct card layouts — FFA
  // (hero strip), 1v1 (duelists), CTF/1FCTF (flag status), Overload (obelisk).
  const botLiveServers =
    activeServers.length === 0
      ? FEATURED_BOT_KEYS.flatMap((key) =>
          allServers.filter(
            (s) => s.source === "hub" && s.key === key && s.online && s.is_live,
          ),
        )
      : [];
  const totalOnline =
    live.servers.size > 0 ? allServers.filter((s) => s.online).length : 0;

  return (
    <section className="landing-section">
      <header className="landing-section__head">
        <h2 className="landing-section__title">
          <span className="landing-section__pulse" aria-hidden /> IN PROGRESS,
          RIGHT NOW.
        </h2>
        <Link to="/servers" className="landing-section__cta">
          All servers <ArrowIcon direction="right" />
        </Link>
      </header>
      {activeServers.length > 0 ? (
        <>
          {activeServers.some((s) => s.is_live) && (
            <p className="landing-section__lead">
              Click a LIVE card to watch — the players will know the Vadrigar
              have arrived.
            </p>
          )}
          <div className="landing-shelf-h">
            <NavScroller scrollClassName="landing-shelf-h__scroll">
              {activeServers.map((server) => (
                <div key={server.server_id} className="landing-shelf-h__slot">
                  {/* landingCard captures the whole card as one click target;
                      no onPlayerClick — profile drill-ins live off-landing. */}
                  <ServerCard
                    server={server}
                    liveness={live.liveness.get(server.server_id)}
                    landingCard
                  />
                </div>
              ))}
            </NavScroller>
          </div>
        </>
      ) : botLiveServers.length > 0 ? (
        <>
          <p className="landing-section__lead">
            No humans fighting right now. The bots didn't get the memo &mdash;
            click a card to watch, or <Link to="/docs">join the fray</Link>.
          </p>
          <div className="landing-shelf-h">
            <NavScroller scrollClassName="landing-shelf-h__scroll">
              {botLiveServers.map((server) => (
                <div key={server.server_id} className="landing-shelf-h__slot">
                  <ServerCard
                    server={server}
                    liveness={live.liveness.get(server.server_id)}
                    landingCard
                  />
                </div>
              ))}
            </NavScroller>
          </div>
        </>
      ) : (
        <div className="landing-quiet">
          <span className="landing-quiet__dot" aria-hidden />
          <p className="landing-quiet__title">STANDING BY</p>
          <p className="landing-quiet__sub">
            No live matches right now — the arena is yours if you want it.
            {totalOnline > 0
              ? ` ${totalOnline} server${totalOnline === 1 ? "" : "s"} standing by.`
              : ""}
          </p>
          <Link to="/servers" className="landing-quiet__cta">
            Browse servers <ArrowIcon direction="right" />
          </Link>
        </div>
      )}
    </section>
  );
}
