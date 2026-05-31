import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { MatchCard } from "../MatchCard";
import { ArrowIcon } from "../ArrowIcon";
import { NavScroller } from "../NavScroller";
import type { MatchSummary } from "../../types";

// How many cards we want on the shelf. Fetch wider than this so we can
// filter down to demo-available matches and still hit the target slot
// count when some recent matches don't have a demo URL yet.
const SHELF_SIZE = 5;
const FETCH_LIMIT = 20;

export function RecentMatchesShelf() {
  const [matches, setMatches] = useState<MatchSummary[]>([]);

  useEffect(() => {
    let cancelled = false;
    fetch(`/api/matches?limit=${FETCH_LIMIT}`)
      .then((res) => (res.ok ? res.json() : []))
      .then((data: MatchSummary[]) => {
        if (cancelled) return;
        const withDemos = (data ?? [])
          .filter((m) => !!m.demo_url)
          .slice(0, SHELF_SIZE);
        setMatches(withDemos);
      })
      .catch(() => {
        if (!cancelled) setMatches([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (matches.length === 0) return null;

  return (
    <section className="landing-section">
      <header className="landing-section__head">
        <h2 className="landing-section__title">THE LAST FEW FIGHTS.</h2>
        <Link to="/matches" className="landing-section__cta">
          All matches <ArrowIcon direction="right" />
        </Link>
      </header>
      <p className="landing-section__lead">
        Recently played. Click any to watch in your browser — no install
        required.
      </p>
      <div className="landing-shelf-h">
        <NavScroller scrollClassName="landing-shelf-h__scroll">
          {matches.map((m) => (
            // Whole-card link to the demo. The lead text promises "click
            // any to watch" — internal links (timestamp, Watch button)
            // and player rows are pointer-events: none'd via the
            // .landing-match-link selector so every pixel of the card
            // resolves to this single action.
            <Link
              key={m.id}
              to={`/matches/${m.id}/demo`}
              state={{ from: "/" }}
              className="landing-shelf-h__slot landing-match-link"
            >
              <MatchCard match={m} />
            </Link>
          ))}
        </NavScroller>
      </div>
    </section>
  );
}
