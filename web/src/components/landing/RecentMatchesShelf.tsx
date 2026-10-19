import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { MatchCard } from "../MatchCard";
import { ArrowIcon } from "../ArrowIcon";
import { NavScroller } from "../NavScroller";
import type { MatchSummary } from "../../types";

// Fetch wider than the shelf so filtering to matches with demos still fills it.
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
        Recently played. Click one to watch it in your browser.
      </p>
      <div className="landing-shelf-h">
        <NavScroller scrollClassName="landing-shelf-h__scroll">
          {matches.map((m) => (
            // .landing-match-link turns off pointer events on the card's own
            // links and player rows, so a click anywhere opens the demo.
            <Link
              key={m.id}
              to={`/matches/${m.id}/demo`}
              reloadDocument
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
