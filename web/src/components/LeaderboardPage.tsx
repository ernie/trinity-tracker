import { useState, useEffect } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ColoredText } from "./ColoredText";
import { PlayerPortrait } from "./PlayerPortrait";
import { PlayerBadge } from "./PlayerBadge";
import { ArrowIcon } from "./ArrowIcon";
import { FlagPair } from "./FlagPair";
import { NavScroller } from "./NavScroller";
import { PeriodSelector } from "./PeriodSelector";

import { GAME_TYPES, type GameTypeFilter } from "../constants/labels";
import { formatGameType } from "./MatchCard";
import { formatNumber, stripVRPrefix } from "../utils";
import type {
  LeaderboardResponse,
  LeaderboardCategory,
  LeaderboardEntry,
  TimePeriod,
} from "../types";

type MedalType =
  | "impressive"
  | "excellent"
  | "humiliation"
  | "capture"
  | "assist"
  | "defend"
  | "victory"
  | "skull"
  | "obelisk";

const CATEGORY_MEDAL: Partial<Record<LeaderboardCategory, MedalType>> = {
  excellents: "excellent",
  impressives: "impressive",
  humiliations: "humiliation",
  captures: "capture",
  assists: "assist",
  defends: "defend",
  victories: "victory",
  skulls_delivered: "skull",
  obelisks_destroyed: "obelisk",
};

const CATEGORY_LABELS: Record<LeaderboardCategory, string> = {
  matches: "Matches",
  kd_ratio: "K/D Ratio",
  frags: "Frags",
  deaths: "Deaths",
  victories: "Victories",
  excellents: "Excellent",
  impressives: "Impressive",
  humiliations: "Humiliation",
  captures: "Captures",
  flag_returns: "Returns",
  assists: "Assists",
  defends: "Defense",
  skulls_delivered: "Skulls",
  obelisks_destroyed: "Obelisks",
};

// Headline-scale titles for the dramatic standard. Uppercase ceremonial
// register; mostly mirrors CATEGORY_LABELS but a few entries (e.g. SKULLS
// DELIVERED, OBELISKS DESTROYED) spell out the full action so the banner
// reads as a sentence-fragment rather than a single noun.
const CATEGORY_TITLE: Record<LeaderboardCategory, string> = {
  matches: "MATCHES",
  kd_ratio: "K/D RATIO",
  frags: "FRAGS",
  deaths: "DEATHS",
  victories: "VICTORIES",
  excellents: "EXCELLENT",
  impressives: "IMPRESSIVE",
  humiliations: "HUMILIATION",
  captures: "CAPTURES",
  flag_returns: "RETURNS",
  assists: "ASSISTS",
  defends: "DEFENSE",
  skulls_delivered: "SKULLS DELIVERED",
  obelisks_destroyed: "OBELISKS DESTROYED",
};

// Per-category eyebrow descriptors — what other regulars whisper about
// the warrior who leads this stat. Drives the small-caps line above the
// title in the standard. Tone: lyrical, ceremonial, slightly elliptical;
// matches the landing-page voice ("the railgun waits, still humming").
const CATEGORY_EYEBROW: Record<LeaderboardCategory, string> = {
  matches: "THE ARENA IS THEIR HOME",
  kd_ratio: "MORE GIVEN THAN TAKEN",
  frags: "STOPPED COUNTING LONG AGO",
  deaths: "NEVER LEARNED TO RETREAT",
  victories: "DEFEAT IS NOT AN OPTION",
  excellents: "ONE IS NEVER ENOUGH",
  impressives: "EVERY SHOT FINDS ITS MARK",
  humiliations: "YOU AREN'T WORTH A BULLET",
  captures: "THEY BEAR THE BURDEN HOME",
  flag_returns: "THEY WON'T LET A BANNER LIE",
  assists: "SHARED THE GLORY GLADLY",
  defends: "THEY DO NOT GIVE GROUND",
  skulls_delivered: "BONES, COLLECTED AND CAST",
  obelisks_destroyed: "PROLIFIC DEMOLITIONISTS",
};

// Banner tagline labels. Spell out the rolling window so visitors aren't
// left guessing whether "Today" means "since midnight" (it doesn't —
// the backend computes asOf - 24h; see storage/sqlite.go getTimePeriodBounds).
const PERIOD_DISPLAY: Record<TimePeriod, string> = {
  all: "ALL-TIME",
  year: "PAST YEAR",
  month: "PAST 30 DAYS",
  week: "PAST 7 DAYS",
  day: "PAST 24 HOURS",
};

// Renders the big emblem for the dramatic standard. Three variants:
//   - medal categories → the corresponding medal PNG
//   - flag_returns    → crossed red+blue base flags (heraldic banners)
//   - matches/K/D/frags/deaths → the Q3 brand mark (skill4.png), since
//     those are the quintessential Quake stats, not gametype-specific
function StandardEmblem({ category }: { category: LeaderboardCategory }) {
  const medalType = CATEGORY_MEDAL[category];
  if (medalType) {
    const src = `/assets/medals/medal_${medalType === "humiliation" ? "gauntlet" : medalType}.png`;
    return <img className="leaderboard-standard__medal-img" src={src} alt="" />;
  }
  if (category === "flag_returns") {
    // Crossed banners: red + blue overlapped at angles. The R+B duality
    // mirrors the obelisk medal's red+blue rings — both TA-mode emblems
    // share the "two teams' colors clashing" motif. Same emblem also
    // marks the flag_returns honor in the honors panel + featured slot.
    return <FlagPair />;
  }
  // matches, kd_ratio, frags, deaths — the Quake-fundamentals, no medal.
  // Uses the flat Q3 brand silhouette as a heraldic emblem.
  return (
    <img
      className="leaderboard-standard__medal-img"
      src="/assets/q3-logo.png"
      alt=""
    />
  );
}

// Base categories available for all game types
const BASE_CATEGORIES: LeaderboardCategory[] = [
  "matches",
  "kd_ratio",
  "frags",
  "deaths",
  "victories",
  "excellents",
  "impressives",
  "humiliations",
];

// CTF-specific categories
const CTF_CATEGORIES: LeaderboardCategory[] = [
  "captures",
  "flag_returns",
  "assists",
  "defends",
];

// 1FCTF categories (no returns)
const ONE_FLAG_CTF_CATEGORIES: LeaderboardCategory[] = [
  "captures",
  "assists",
  "defends",
];

const OVERLOAD_CATEGORIES: LeaderboardCategory[] = [
  "obelisks_destroyed",
  "defends",
];
const HARVESTER_CATEGORIES: LeaderboardCategory[] = [
  "skulls_delivered",
  "assists",
  "defends",
];

function getCategoriesForGameType(
  gameType: GameTypeFilter,
): LeaderboardCategory[] {
  switch (gameType) {
    case "ctf":
      return [...BASE_CATEGORIES, ...CTF_CATEGORIES];
    case "1fctf":
      return [...BASE_CATEGORIES, ...ONE_FLAG_CTF_CATEGORIES];
    case "overload":
      return [...BASE_CATEGORIES, ...OVERLOAD_CATEGORIES];
    case "harvester":
      return [...BASE_CATEGORIES, ...HARVESTER_CATEGORIES];
    default:
      // 'all' aggregates every objective-mode category.
      return gameType === "all"
        ? [
            ...BASE_CATEGORIES,
            ...CTF_CATEGORIES,
            "skulls_delivered",
            "obelisks_destroyed",
          ]
        : BASE_CATEGORIES;
  }
}

// formatSnapshotTime renders an as_of timestamp for the snapshot
// banner. Falls back to the raw string if it doesn't parse — the API
// already 400s on garbage so this is just defensive.
function formatSnapshotTime(asOf: string): string {
  const d = new Date(asOf);
  if (isNaN(d.getTime())) return asOf;
  return d
    .toISOString()
    .replace("T", " ")
    .replace(/:\d{2}\.\d+Z$/, " UTC");
}

export function LeaderboardPage() {
  const [gameType, setGameType] = useState<GameTypeFilter>("all");
  const [category, setCategory] = useState<LeaderboardCategory>("matches");
  const [period, setPeriod] = useState<TimePeriod>("all");
  const [data, setData] = useState<LeaderboardResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // ?as_of=<RFC3339> pins the leaderboard's upper bound for snapshot
  // links (e.g. the Discord digest's footer URL). When present we pass
  // it through to the API and show a banner so visitors know they're
  // looking at history. The period and category selectors stay
  // functional — flipping them keeps the same anchor.
  const [searchParams, setSearchParams] = useSearchParams();
  const asOf = searchParams.get("as_of") ?? "";

  const availableCategories = getCategoriesForGameType(gameType);

  // Effective category — when a game type doesn't support the persisted
  // category, fall back to "frags" without ever storing the bad value.
  const effectiveCategory = availableCategories.includes(category)
    ? category
    : "frags";

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    setError(null);

    const gameTypeParam = gameType !== "all" ? `&game_type=${gameType}` : "";
    const asOfParam = asOf ? `&as_of=${encodeURIComponent(asOf)}` : "";
    fetch(
      `/api/stats/leaderboard?category=${effectiveCategory}&period=${period}&limit=10${gameTypeParam}${asOfParam}`,
    )
      .then((res) => {
        if (!res.ok) throw new Error("Failed to load leaderboard");
        return res.json();
      })
      .then((data) => setData(data))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [effectiveCategory, period, gameType, asOf]);

  function exitSnapshot() {
    const next = new URLSearchParams(searchParams);
    next.delete("as_of");
    setSearchParams(next);
  }

  return (
    <div className="leaderboard-page">
      {asOf && (
        <div className="leaderboard-snapshot-banner">
          <span>Snapshot · {formatSnapshotTime(asOf)}</span>
          <button
            type="button"
            className="leaderboard-snapshot-exit"
            onClick={exitSnapshot}
          >
            <ArrowIcon direction="left" /> Live
          </button>
        </div>
      )}

      <div className="leaderboard-filters">
        <div className="leaderboard-filters__game-type">
          <NavScroller scrollClassName="filter-chips__scroll">
            <div className="filter-chips__strip">
              <button
                key="all"
                className={`game-type-btn ${gameType === "all" ? "active" : ""}`}
                onClick={() => setGameType("all")}
              >
                All
              </button>
              {GAME_TYPES.map((gt) => (
                <button
                  key={gt}
                  className={`game-type-btn ${gameType === gt ? "active" : ""}`}
                  onClick={() => setGameType(gt)}
                >
                  {formatGameType(gt)}
                </button>
              ))}
            </div>
          </NavScroller>
        </div>
        <div className="leaderboard-filters__period">
          <PeriodSelector period={period} onChange={setPeriod} />
        </div>
      </div>

      <div className="leaderboard-body">
        <NavScroller scrollClassName="category-strip__scroll">
          <nav className="category-strip">
            {availableCategories.map((cat) => (
              <button
                key={cat}
                className={`category-tab ${effectiveCategory === cat ? "active" : ""}`}
                onClick={() => setCategory(cat)}
              >
                {CATEGORY_LABELS[cat]}
              </button>
            ))}
          </nav>
        </NavScroller>

        <div className="leaderboard-main">
          <header key={effectiveCategory} className="leaderboard-standard">
            <div className="leaderboard-standard__ribbon" aria-hidden />
            <div className="leaderboard-standard__emblem">
              <div className="leaderboard-standard__halo" aria-hidden />
              <StandardEmblem category={effectiveCategory} />
            </div>
            <div className="leaderboard-standard__text">
              <p className="leaderboard-standard__eyebrow">
                <span>{CATEGORY_EYEBROW[effectiveCategory]}</span>
              </p>
              <h2 className="leaderboard-standard__title">
                {CATEGORY_TITLE[effectiveCategory]}
              </h2>
              <p className="leaderboard-standard__tagline">
                {PERIOD_DISPLAY[period]} ·{" "}
                {gameType === "all"
                  ? "ALL MODES"
                  : formatGameType(gameType).toUpperCase()}
              </p>
            </div>
          </header>

          <div className="leaderboard-content">
            {loading ? (
              <div className="stats-loading">Loading leaderboard...</div>
            ) : error ? (
              <div className="stats-error">{error}</div>
            ) : data && data.entries && data.entries.length > 0 ? (
              <LeaderboardGrid
                entries={data.entries}
                category={effectiveCategory}
              />
            ) : (
              <div className="leaderboard-empty">
                No data available for this selection
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

interface LeaderboardGridProps {
  entries: LeaderboardEntry[];
  category: LeaderboardCategory;
}

const CORE_STATS_CATEGORIES = [
  "matches",
  "kd_ratio",
  "frags",
  "deaths",
] as const;

// Renders the sorted-by stat as the card's headline number. For
// kd_ratio shows two decimals, everything else passes through
// formatNumber.
function getPrimary(
  entry: LeaderboardEntry,
  category: LeaderboardCategory,
): string {
  switch (category) {
    case "matches":
      return formatNumber(entry.completed_matches);
    case "kd_ratio":
      return entry.kd_ratio.toFixed(2);
    case "frags":
      return formatNumber(entry.total_frags);
    case "deaths":
      return formatNumber(entry.total_deaths);
    case "victories":
      return formatNumber(entry.victories);
    case "excellents":
      return formatNumber(entry.excellents);
    case "impressives":
      return formatNumber(entry.impressives);
    case "humiliations":
      return formatNumber(entry.humiliations);
    case "captures":
      return formatNumber(entry.captures);
    case "flag_returns":
      return formatNumber(entry.flag_returns);
    case "assists":
      return formatNumber(entry.assists);
    case "defends":
      return formatNumber(entry.defends);
    case "skulls_delivered":
      return formatNumber(entry.skulls_delivered);
    case "obelisks_destroyed":
      return formatNumber(entry.obelisks_destroyed);
    default:
      return "";
  }
}

function LeaderboardGrid({ entries, category }: LeaderboardGridProps) {
  const isCoreStats = CORE_STATS_CATEGORIES.includes(
    category as (typeof CORE_STATS_CATEGORIES)[number],
  );

  return (
    <div className="leaderboard-grid">
      {entries.map((entry, index) => {
        const isTop = index < 3;
        return (
          <Link
            key={entry.player.id}
            to={`/players/${entry.player.id}`}
            className={`leaderboard-card${isTop ? " top" : ""}${isTop ? ` top-${index + 1}` : ""}`}
          >
            <span className={`leaderboard-card__rank${isTop ? " top" : ""}`}>
              #{index + 1}
            </span>
            <span className="leaderboard-card__avatar">
              <PlayerPortrait model={entry.player.model} size="lg" />
              <PlayerBadge
                isVerified={entry.player.is_verified}
                isAdmin={entry.player.is_admin}
                isVR={entry.player.is_vr}
                size="sm"
              />
            </span>
            <span className="leaderboard-card__name">
              <ColoredText
                text={
                  entry.player.is_vr
                    ? stripVRPrefix(entry.player.name)
                    : entry.player.name
                }
              />
            </span>
            <span className="leaderboard-card__stat-label">
              {CATEGORY_LABELS[category]}
            </span>
            <span className="leaderboard-card__stat">
              {getPrimary(entry, category)}
            </span>
            {isCoreStats && (
              // Show the other three core stats as a compact secondary row
              // so each card carries the full stat picture, not just the
              // sorted column.
              <div className="leaderboard-card__secondary">
                {category !== "matches" && (
                  <span
                    title={
                      entry.uncompleted_matches > 0
                        ? `${formatNumber(entry.completed_matches)} completed, ${formatNumber(entry.uncompleted_matches)} incomplete`
                        : undefined
                    }
                  >
                    <i>M</i>
                    {formatNumber(entry.completed_matches)}
                  </span>
                )}
                {category !== "kd_ratio" && (
                  <span>
                    <i>K/D</i>
                    {entry.kd_ratio.toFixed(2)}
                  </span>
                )}
                {category !== "frags" && (
                  <span>
                    <i>K</i>
                    {formatNumber(entry.total_frags)}
                  </span>
                )}
                {category !== "deaths" && (
                  <span>
                    <i>D</i>
                    {formatNumber(entry.total_deaths)}
                  </span>
                )}
              </div>
            )}
          </Link>
        );
      })}
    </div>
  );
}
