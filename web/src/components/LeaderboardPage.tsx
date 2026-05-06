import { useState, useEffect } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ColoredText } from "./ColoredText";
import { PlayerPortrait } from "./PlayerPortrait";
import { PlayerBadge } from "./PlayerBadge";
import { FlagIcon } from "./FlagIcon";
import { MedalIcon } from "./MedalIcon";
import { Header } from "./Header";
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
  | "victory";

const CATEGORY_MEDAL: Partial<Record<LeaderboardCategory, MedalType>> = {
  excellents: "excellent",
  impressives: "impressive",
  humiliations: "humiliation",
  captures: "capture",
  assists: "assist",
  defends: "defend",
  victories: "victory",
};

function CategoryIcon({ category }: { category: LeaderboardCategory }) {
  const medalType = CATEGORY_MEDAL[category];
  if (medalType) {
    return <MedalIcon type={medalType} showCount={false} />;
  }
  if (category === "flag_returns") {
    return <FlagIcon team="red" status="base" size="sm" />;
  }
  return null;
}

const CATEGORY_LABELS: Record<LeaderboardCategory, string> = {
  matches: "Matches",
  kd_ratio: "K/D",
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
};

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

// Overload categories (defense only)
const OVERLOAD_CATEGORIES: LeaderboardCategory[] = ["defends"];

// Harvester categories (assists + defense)
const HARVESTER_CATEGORIES: LeaderboardCategory[] = ["assists", "defends"];

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
      // For 'all', show everything; for non-CTF modes, just base categories
      return gameType === "all"
        ? [...BASE_CATEGORIES, ...CTF_CATEGORIES]
        : BASE_CATEGORIES;
  }
}

// formatSnapshotTime renders an as_of timestamp for the snapshot
// banner. Falls back to the raw string if it doesn't parse — the API
// already 400s on garbage so this is just defensive.
function formatSnapshotTime(asOf: string): string {
  const d = new Date(asOf);
  if (isNaN(d.getTime())) return asOf;
  return d.toISOString().replace("T", " ").replace(/:\d{2}\.\d+Z$/, " UTC");
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
  const effectiveCategory = availableCategories.includes(category) ? category : "frags";

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    setError(null);

    const gameTypeParam = gameType !== "all" ? `&game_type=${gameType}` : "";
    const asOfParam = asOf ? `&as_of=${encodeURIComponent(asOf)}` : "";
    fetch(
      `/api/stats/leaderboard?category=${effectiveCategory}&period=${period}&limit=50${gameTypeParam}${asOfParam}`,
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
      <Header title="Leaderboard" className="leaderboard-header" />

      {asOf && (
        <div className="leaderboard-snapshot-banner">
          <span>📸 Snapshot · {formatSnapshotTime(asOf)}</span>
          <button type="button" className="leaderboard-snapshot-exit" onClick={exitSnapshot}>
            ← back to live
          </button>
        </div>
      )}

      <div className="filter-row">
        <div className="game-type-selector">
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
      </div>

      <div className="category-selector">
        {availableCategories.map((cat) => (
          <button
            key={cat}
            className={`category-btn ${effectiveCategory === cat ? "active" : ""}`}
            onClick={() => setCategory(cat)}
          >
            <CategoryIcon category={cat} />
            {CATEGORY_LABELS[cat]}
          </button>
        ))}
      </div>

      <PeriodSelector period={period} onChange={setPeriod} />

      <div className="leaderboard-content">
        {loading ? (
          <div className="stats-loading">Loading leaderboard...</div>
        ) : error ? (
          <div className="stats-error">{error}</div>
        ) : data && data.entries && data.entries.length > 0 ? (
          <LeaderboardTable
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
  );
}

interface LeaderboardTableProps {
  entries: LeaderboardEntry[];
  category: LeaderboardCategory;
}

const CORE_STATS_CATEGORIES = [
  "matches",
  "kd_ratio",
  "frags",
  "deaths",
] as const;

function LeaderboardTable({
  entries,
  category,
}: LeaderboardTableProps) {
  const isCoreStats = CORE_STATS_CATEGORIES.includes(
    category as (typeof CORE_STATS_CATEGORIES)[number],
  );

  const getAwardValue = (entry: LeaderboardEntry): string => {
    switch (category) {
      case "captures":
        return formatNumber(entry.captures);
      case "flag_returns":
        return formatNumber(entry.flag_returns);
      case "assists":
        return formatNumber(entry.assists);
      case "impressives":
        return formatNumber(entry.impressives);
      case "excellents":
        return formatNumber(entry.excellents);
      case "humiliations":
        return formatNumber(entry.humiliations);
      case "defends":
        return formatNumber(entry.defends);
      case "victories":
        return formatNumber(entry.victories);
      default:
        return "";
    }
  };

  const colClass = (col: string) =>
    `stat-col ${category === col ? "sorted-col" : ""}`;

  if (isCoreStats) {
    return (
      <table className="leaderboard-table">
        <thead>
          <tr>
            <th className="rank-col">#</th>
            <th className="player-col">Player</th>
            <th className={colClass("matches")}>Matches</th>
            <th className={colClass("kd_ratio")}>K/D</th>
            <th className={colClass("frags")}>Frags</th>
            <th className={colClass("deaths")}>Deaths</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry, index) => (
            <tr
              key={entry.player.id}
              className={index < 3 ? `top-${index + 1}` : ""}
            >
              <td className="rank-col">{index + 1}</td>
              <td className="player-col">
                <span className="player-name">
                  <PlayerPortrait model={entry.player.model} size="sm" />
                  <PlayerBadge isVerified={entry.player.is_verified} isAdmin={entry.player.is_admin} isVR={entry.player.is_vr} />
                  <Link to={`/players/${entry.player.id}`}>
                    <ColoredText text={entry.player.is_vr ? stripVRPrefix(entry.player.name) : entry.player.name} />
                  </Link>
                </span>
              </td>
              <td className={colClass("matches")} title={
                entry.uncompleted_matches > 0
                  ? `${formatNumber(entry.completed_matches)} completed, ${formatNumber(entry.uncompleted_matches)} incomplete`
                  : undefined
              }>
                {formatNumber(entry.completed_matches)}
              </td>
              <td className={colClass("kd_ratio")}>
                {entry.kd_ratio.toFixed(2)}
              </td>
              <td className={colClass("frags")}>{formatNumber(entry.total_frags)}</td>
              <td className={colClass("deaths")}>{formatNumber(entry.total_deaths)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    );
  }

  // Award categories - single value column
  return (
    <table className="leaderboard-table">
      <thead>
        <tr>
          <th className="rank-col">#</th>
          <th className="player-col">Player</th>
          <th className="stat-col sorted-col">{CATEGORY_LABELS[category]}</th>
        </tr>
      </thead>
      <tbody>
        {entries.map((entry, index) => (
          <tr
            key={entry.player.id}
            className={index < 3 ? `top-${index + 1}` : ""}
          >
            <td className="rank-col">{index + 1}</td>
            <td className="player-col">
              <span className="player-name">
                <PlayerPortrait model={entry.player.model} size="sm" />
                <PlayerBadge isVerified={entry.player.is_verified} isAdmin={entry.player.is_admin} isVR={entry.player.is_vr} />
                <Link to={`/players/${entry.player.id}`}>
                  <ColoredText text={entry.player.is_vr ? stripVRPrefix(entry.player.name) : entry.player.name} />
                </Link>
              </span>
            </td>
            <td className="stat-col sorted-col">{getAwardValue(entry)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
