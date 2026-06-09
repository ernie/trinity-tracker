import type React from "react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  useFeaturedMatches,
  pickRandomFeatured,
} from "../../hooks/useFeaturedMatches";
import { useLiveData } from "../../contexts/LiveDataContext";
import { ArrowIcon } from "../ArrowIcon";
import { DISCORD_INVITE_URL } from "../../constants/discord";

// Each door wears a faded, rotated in-game glyph at its bottom-right —
// echoes the "medal whisper" pattern on .stat-item, ember-shifted to
// blend into the door's hover bloom. Rotation is the same -20° as
// the player-page convention so the two pages share one signature.
// Optional `opacity` overrides the 0.22 default for delicate glyphs
// (the medal laurel is line-art and reads dimmer than the others).
type GlyphStyle = React.CSSProperties & {
  "--glyph-url"?: string;
  "--glyph-opacity"?: string;
};
const glyph = (src: string, opacity?: number): GlyphStyle => {
  const style: GlyphStyle = { "--glyph-url": `url('${src}')` };
  if (opacity !== undefined) style["--glyph-opacity"] = String(opacity);
  return style;
};

export function DoorsSection() {
  const { ids } = useFeaturedMatches();
  const { servers } = useLiveData();

  // The tap is always live (even bot-only), so prefer humans when present.
  const livePool = useMemo(() => {
    const tapped = [...servers.values()].filter((s) => s.is_live);
    const withHumans = tapped.filter((s) => s.human_count > 0);
    return withHumans.length > 0 ? withHumans : tapped;
  }, [servers]);

  // One stable coin flip per mount, so live and demo each get ~half the visitors.
  const seed = useState(() => Math.random())[0];
  const liveServer =
    livePool.length > 0 && (ids.length === 0 || seed < 0.5)
      ? livePool[Math.floor(seed * livePool.length)]
      : null;
  const watchTo = liveServer
    ? `/tv/${liveServer.source}/${liveServer.key}`
    : "/matches";

  const handleWatch = (e: React.MouseEvent) => {
    // Both boot the WASM engine, so both need a full-document load: live via
    // the native href, demo after picking a random featured match.
    if (liveServer) return;
    e.preventDefault();
    const id = pickRandomFeatured(ids);
    window.location.assign(id !== null ? `/matches/${id}/demo` : "/matches");
  };

  return (
    <section className="landing-section landing-doors">
      <div className="landing-doors__grid">
        <Link
          to="/servers"
          className="landing-door"
          style={glyph("/assets/landing/door-find.png")}
        >
          <span className="landing-door__glyph" aria-hidden="true" />
          <h3 className="landing-door__title">Find a fight</h3>
          <p className="landing-door__desc">
            Live servers, current scores, who&rsquo;s on.
          </p>
          <span className="landing-door__arrow">
            All servers <ArrowIcon direction="right" />
          </span>
        </Link>

        <a
          href={watchTo}
          onClick={handleWatch}
          className="landing-door"
          style={glyph("/assets/landing/door-watch.png", 0.3)}
        >
          <span className="landing-door__glyph" aria-hidden="true" />
          <h3 className="landing-door__title">Watch a fight</h3>
          <p className="landing-door__desc">
            As it happens, or replayed frame by frame.
          </p>
          <span className="landing-door__arrow">
            {liveServer ? "Live now" : "Featured demo"}{" "}
            <ArrowIcon direction="right" />
          </span>
        </a>

        <Link
          to="/leaderboard"
          className="landing-door"
          style={glyph("/assets/landing/door-respects.png")}
        >
          <span className="landing-door__glyph" aria-hidden="true" />
          <h3 className="landing-door__title">Pay respects</h3>
          <p className="landing-door__desc">
            Weekly rankings, all-time legends, every mode.
          </p>
          <span className="landing-door__arrow">
            Leaderboard <ArrowIcon direction="right" />
          </span>
        </Link>

        <a
          href={DISCORD_INVITE_URL}
          target="_blank"
          rel="noreferrer"
          className="landing-door"
          style={glyph("/assets/landing/door-discord.png")}
        >
          <span className="landing-door__glyph" aria-hidden="true" />
          <h3 className="landing-door__title">Talk to the regulars</h3>
          <p className="landing-door__desc">
            Where everyone is when they&rsquo;re not in a server.
          </p>
          <span className="landing-door__arrow">
            Discord <ArrowIcon direction="right" />
          </span>
        </a>
      </div>
    </section>
  );
}
