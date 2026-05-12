import type React from "react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { useLiveData } from "../../contexts/LiveDataContext";
import { plural, formatFragTime } from "./format";
import { HeroHeader } from "./HeroHeader";
import { ArrowIcon } from "../ArrowIcon";
import { pickHeroVariant } from "./heroVariants";

// Preserve the non-breaking-space treatment the static headline used
// (`WELCOME&nbsp;BACK&nbsp;TO`) so dynamic headline lines don't wrap
// mid-phrase on narrow viewports.
const nbsp = (s: string) => s.replace(/ /g, " ");

export function HeroSection() {
  const live = useLiveData();
  const humans = live.activeHumanPlayersCount;
  const arenas = live.activeServersCount;
  // Pick once per mount; useState lazy-init means the random roll is
  // stable across re-renders. Route navigation that unmounts and remounts
  // the landing will pick a fresh variant — matches "random per visit".
  const [variant] = useState(pickHeroVariant);

  // Pulse line 1: busy / quiet / pre-WS variants.
  let pulse: React.ReactNode;
  if (!live.isConnected) {
    pulse = <span>the arena is open.</span>;
  } else if (humans === 0) {
    pulse = <span>the arena is waiting for you.</span>;
  } else {
    pulse = (
      <span>
        <span className="landing-hero__pulse-dot" aria-hidden /> {humans}{" "}
        {plural(humans, "player", "players")} fragging across {arenas}{" "}
        {plural(arenas, "arena", "arenas")}
      </span>
    );
  }

  // Pulse line 2: matches today + frag duration. Stubbed in v1 — the underlying
  // values aren't on LiveDataContext yet. When `matchesToday === 0` the line
  // gracefully renders empty, so the hero still looks complete.
  // TODO(landing-page): wire matchesToday + fragSecondsToday from a derived hook
  //   or add the fields to LiveDataContext (compute from recentMatches today UTC,
  //   filtered to has_human_player && demo_available).
  const matchesToday = 0;
  const fragTime = formatFragTime(0);
  const pulse2 =
    matchesToday > 0
      ? `${matchesToday} matches recorded today${fragTime ? ` · ${fragTime}` : ""}`
      : "";

  return (
    <section className="landing-hero">
      <picture className="landing-hero__bg" aria-hidden>
        <source
          type="image/avif"
          srcSet="
            /assets/landing/wallpaper-1200.avif 1200w,
            /assets/landing/wallpaper-1920.avif 1920w,
            /assets/landing/wallpaper-2880.avif 2880w"
          sizes="100vw"
        />
        <source
          type="image/webp"
          srcSet="
            /assets/landing/wallpaper-1200.webp 1200w,
            /assets/landing/wallpaper-1920.webp 1920w,
            /assets/landing/wallpaper-2880.webp 2880w"
          sizes="100vw"
        />
        <img
          src="/assets/landing/wallpaper-wqhd.png"
          alt=""
          fetchPriority="high"
        />
      </picture>

      <HeroHeader />

      <div className="landing-hero__main">
        <div className="landing-hero__kicker">
          <span className="landing-hero__rule" aria-hidden />
          <span>EST. 2026 · TRINITY · QUAKE III</span>
          <span className="landing-hero__rule" aria-hidden />
        </div>

        <h1 className="landing-hero__headline">
          {nbsp(variant.headline1)}
          <br />
          {nbsp(variant.headline2)}
        </h1>

        <p className="landing-hero__subhead">{variant.subhead}</p>

        <div className="landing-hero__pulse" aria-live="polite">
          <div className="landing-hero__pulse-line">{pulse}</div>
          {pulse2 && <div className="landing-hero__pulse-line2">{pulse2}</div>}
        </div>

        <div className="landing-hero__cta-row">
          <Link to="/docs" className="landing-cta-primary">
            Enter the arena
          </Link>
          <Link to="/leaderboard" className="landing-cta-secondary">
            See the leaderboard <ArrowIcon direction="right" />
          </Link>
        </div>
      </div>

      <div className="landing-hero__scroll-hint" aria-hidden>
        Scroll <ArrowIcon direction="down" />
      </div>

      <div className="landing-hero__wallpaper" tabIndex={0}>
        <span className="landing-hero__wallpaper-trigger">Wallpaper</span>
        <div className="landing-hero__wallpaper-popup" role="menu">
          <div className="landing-hero__wallpaper-popup-label">
            Download wallpaper
          </div>
          <a
            className="landing-hero__wallpaper-link"
            role="menuitem"
            href="/assets/landing/wallpaper-qhd.png"
            download="trinity-wallpaper-2560x1440.png"
          >
            <span>QHD</span>
            <span className="landing-hero__wallpaper-link-meta">
              2560 × 1440
            </span>
          </a>
          <a
            className="landing-hero__wallpaper-link"
            role="menuitem"
            href="/assets/landing/wallpaper-4k.png"
            download="trinity-wallpaper-3840x2160.png"
          >
            <span>4K</span>
            <span className="landing-hero__wallpaper-link-meta">
              3840 × 2160
            </span>
          </a>
          <a
            className="landing-hero__wallpaper-link"
            role="menuitem"
            href="/assets/landing/wallpaper-wqhd.png"
            download="trinity-wallpaper-3440x1440.png"
          >
            <span>Ultrawide</span>
            <span className="landing-hero__wallpaper-link-meta">
              3440 × 1440
            </span>
          </a>
          <a
            className="landing-hero__wallpaper-link"
            role="menuitem"
            href="/assets/landing/wallpaper-uw5k.png"
            download="trinity-wallpaper-5160x2160.png"
          >
            <span>5K Ultrawide</span>
            <span className="landing-hero__wallpaper-link-meta">
              5160 × 2160
            </span>
          </a>
          <a
            className="landing-hero__wallpaper-link"
            role="menuitem"
            href="/assets/landing/wallpaper-iphone.png"
            download="trinity-wallpaper-1320x2868.png"
          >
            <span>iPhone</span>
            <span className="landing-hero__wallpaper-link-meta">
              1320 × 2868
            </span>
          </a>
        </div>
      </div>
    </section>
  );
}
