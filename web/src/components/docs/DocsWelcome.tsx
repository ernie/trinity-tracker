import type React from "react";
import { Link } from "react-router-dom";
import { DISCORD_INVITE_URL } from "../../constants/discord";
import { useLiveData } from "../../contexts/LiveDataContext";
import { DocsH2 } from "./DocsH2";
import { ArrowIcon } from "../ArrowIcon";
import {
  useFeaturedMatches,
  pickRandomFeatured,
} from "../../hooks/useFeaturedMatches";

// New /docs index. Replaces the previous redirect to
// /docs/getting-started. First-time visitors see this after picking
// a platform; the persona cards route them to the right deeper
// surface.
export function DocsWelcome() {
  const { ids } = useFeaturedMatches();
  const live = useLiveData();

  // Most-humans-first live broadcast for the "watch live right now" link;
  // bots keep the arena on air, so this is rarely empty.
  const liveServer = Array.from(live.servers.values())
    .filter((s) => s.online && s.is_live)
    .sort((a, b) => (b.human_count ?? 0) - (a.human_count ?? 0))[0];

  // Mirrors the landing page's "Watch a fight" door — picks a random
  // featured match and opens its demo player. Falls back to /matches
  // if the featured pool is empty (e.g., a fresh deploy with nothing
  // flagged yet).
  const handleWatchFeatured = (e: React.MouseEvent) => {
    // Full-document load so the demo player's WASM engine boots fresh.
    e.preventDefault();
    const id = pickRandomFeatured(ids);
    window.location.assign(id !== null ? `/matches/${id}/demo` : "/matches");
  };

  return (
    <>
      <div className="about-section docs-welcome__hero">
        <h1 className="docs-welcome__title">Welcome to Trinity</h1>
        <p className="docs-welcome__lead">
          Trinity is Quake 3 with modern conveniences — VR, voice chat, and
          shared stats — built on top of the original game you know. It's
          backwards-compatible with vanilla Q3 servers, so installing it never
          costs you the rest of the community.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="where-to-start">Where to start</DocsH2>
        <div className="docs-welcome__personas">
          <Link to="/docs/install" className="docs-welcome__persona-card">
            <span className="docs-welcome__persona-title">Install</span>
            <span className="docs-welcome__persona-teaser">
              You're just 3 steps away from playing Trinity.
            </span>
            <ArrowIcon
              direction="right"
              size={14}
              className="docs-welcome__persona-arrow"
            />
          </Link>

          <Link to="/docs/play" className="docs-welcome__persona-card">
            <span className="docs-welcome__persona-title">Play</span>
            <span className="docs-welcome__persona-teaser">
              What you'll want to know to get the most out of Trinity.
            </span>
            <ArrowIcon
              direction="right"
              size={14}
              className="docs-welcome__persona-arrow"
            />
          </Link>

          <Link to="/docs/admin" className="docs-welcome__persona-card">
            <span className="docs-welcome__persona-title">Host</span>
            <span className="docs-welcome__persona-teaser">
              Connect your Trinity server to the tracker network.
            </span>
            <ArrowIcon
              direction="right"
              size={14}
              className="docs-welcome__persona-arrow"
            />
          </Link>
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="watch-featured">Watch a match</DocsH2>
        <p>
          Curious before you commit?{" "}
          {liveServer ? (
            <>
              <a
                href={`/tv/${liveServer.source}/${liveServer.key}`}
                rel="noopener"
              >
                Watch a live match right now
              </a>{" "}
              — the arena is on air around the clock, no install required. You
              can also{" "}
              <a href="/matches" onClick={handleWatchFeatured}>
                watch a featured match
              </a>{" "}
              — a curated fight, replayed frame by frame.
            </>
          ) : (
            <>
              <a href="/matches" onClick={handleWatchFeatured}>
                Watch a featured match
              </a>{" "}
              — replays a real Trinity match frame by frame, no install
              required. And when a server card shows a pulsing{" "}
              <strong>LIVE</strong> pill, you can watch a match as it happens,
              the same way.
            </>
          )}
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="get-help">Get help, get involved</DocsH2>
        <p>
          Stuck on something or just want to say hi?{" "}
          <a href={DISCORD_INVITE_URL}>Trinity Discord</a> is the place.
        </p>
      </div>
    </>
  );
}
