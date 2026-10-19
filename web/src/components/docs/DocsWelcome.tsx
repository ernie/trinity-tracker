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

export function DocsWelcome() {
  const { ids } = useFeaturedMatches();
  const live = useLiveData();

  // Bots keep a broadcast on air, so this is rarely empty.
  const liveServer = Array.from(live.servers.values())
    .filter((s) => s.online && s.is_live)
    .sort((a, b) => (b.human_count ?? 0) - (a.human_count ?? 0))[0];

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
          Trinity is Quake III Arena brought up to date, with VR, voice chat and
          stats that follow you between servers. It still connects to standard
          Quake III servers. Start with Install. Play has everything else you
          need.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="where-to-start">Where to start</DocsH2>
        <div className="docs-welcome__personas">
          <Link to="/docs/install" className="docs-welcome__persona-card">
            <span className="docs-welcome__persona-title">Install</span>
            <span className="docs-welcome__persona-teaser">
              Get Quake III Arena and run the Trinity Installer.
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
              Accounts, voice chat, VR, the HUD, game modes, the grapple, and
              demos.
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
          {liveServer ? (
            <>
              <a
                href={`/tv/${liveServer.source}/${liveServer.key}`}
                rel="noopener"
              >
                Watch a live match right now
              </a>{" "}
              in your browser. Bots keep a server on air around the clock. You
              can also{" "}
              <a href="/matches" onClick={handleWatchFeatured}>
                watch a featured match
              </a>
              , picked by hand and replayed frame by frame.
            </>
          ) : (
            <>
              <a href="/matches" onClick={handleWatchFeatured}>
                Watch a featured match
              </a>{" "}
              to replay a real Trinity match frame by frame in your browser.
              When a server card shows a pulsing <strong>LIVE</strong> pill, you
              can watch that match as it happens the same way.
            </>
          )}
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="get-help">Get help</DocsH2>
        <p>
          Ask questions in <a href={DISCORD_INVITE_URL}>the Trinity Discord</a>.
        </p>
      </div>
    </>
  );
}
