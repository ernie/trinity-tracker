import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import { AppLogo } from "./AppLogo";
import { ColoredText } from "./ColoredText";
import { PlayerPortrait } from "./PlayerPortrait";
import { PlayerBadge } from "./PlayerBadge";
import { BotBadge } from "./BotBadge";
import { FlagIcon } from "./FlagIcon";
import { MedalIcon } from "./MedalIcon";
import { PageNav } from "./PageNav";
import { LoginForm } from "./LoginForm";
import { UserManagement } from "./UserManagement";
import { PeriodSelector } from "./PeriodSelector";
import { useAuth } from "../hooks/useAuth";
import { GAME_TYPE_LABELS, type GameTypeFilter } from "../constants/labels";
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

interface LeaderboardPageProps {
  botsOnly?: boolean;
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

export function LeaderboardPage({ botsOnly = false }: LeaderboardPageProps) {
  const { auth, login, logout } = useAuth();
  const [gameType, setGameType] = useState<GameTypeFilter>("all");
  const [category, setCategory] = useState<LeaderboardCategory>("matches");
  const [period, setPeriod] = useState<TimePeriod>("all");
  const [data, setData] = useState<LeaderboardResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showUserManagement, setShowUserManagement] = useState(false);

  const availableCategories = getCategoriesForGameType(gameType);

  // Reset category if it's no longer available for the selected game type
  useEffect(() => {
    if (!availableCategories.includes(category)) {
      setCategory("frags");
    }
  }, [gameType, availableCategories, category]);

  useEffect(() => {
    setLoading(true);
    setError(null);

    const botsParam = botsOnly ? "&bots_only=true" : "";
    const gameTypeParam = gameType !== "all" ? `&game_type=${gameType}` : "";
    fetch(
      `/api/stats/leaderboard?category=${category}&period=${period}&limit=50${botsParam}${gameTypeParam}`,
    )
      .then((res) => {
        if (!res.ok) throw new Error("Failed to load leaderboard");
        return res.json();
      })
      .then((data) => setData(data))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [category, period, botsOnly, gameType]);

  return (
    <div className="leaderboard-page">
      <header className="leaderboard-header">
        <h1>
          <AppLogo />
          {botsOnly ? "Bot Hall of Shame" : "Leaderboard"}
        </h1>
        <PageNav />
        <div className="auth-section">
          {auth.isAuthenticated ? (
            <div className="user-info">
              <Link to="/account" className="username-link">{auth.username}</Link>
              {auth.isAdmin && (
                <button onClick={() => setShowUserManagement(true)} className="admin-btn">Users</button>
              )}
              <button onClick={logout} className="logout-btn">Logout</button>
            </div>
          ) : (
            <LoginForm onLogin={(username, password) => login({ username, password })} />
          )}
        </div>
      </header>

      <div className="filter-row">
        <div className="game-type-selector">
          {(Object.keys(GAME_TYPE_LABELS) as GameTypeFilter[]).map((gt) => (
            <button
              key={gt}
              className={`game-type-btn ${gameType === gt ? "active" : ""}`}
              onClick={() => setGameType(gt)}
            >
              {GAME_TYPE_LABELS[gt]}
            </button>
          ))}
        </div>
      </div>

      <div className="category-selector">
        {availableCategories.map((cat) => (
          <button
            key={cat}
            className={`category-btn ${category === cat ? "active" : ""}`}
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
            category={category}
            botsOnly={botsOnly}
          />
        ) : (
          <div className="leaderboard-empty">
            No data available for this selection
          </div>
        )}
      </div>

      {showUserManagement && auth.isAdmin && auth.token && (
        <UserManagement
          token={auth.token}
          currentUsername={auth.username!}
          onClose={() => setShowUserManagement(false)}
        />
      )}
    </div>
  );
}

interface LeaderboardTableProps {
  entries: LeaderboardEntry[];
  category: LeaderboardCategory;
  botsOnly: boolean;
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
  botsOnly,
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
                <PlayerPortrait model={entry.player.model} size="sm" />
                {botsOnly && <BotBadge isBot skill={entry.player.skill || 5} />}
                {!botsOnly && <PlayerBadge playerId={entry.player.id} isVR={entry.player.is_vr} />}
                <Link to={`/players/${entry.player.id}`}>
                  <ColoredText text={entry.player.is_vr ? stripVRPrefix(entry.player.name) : entry.player.name} />
                </Link>
              </td>
              <td className={colClass("matches")} title={
                entry.uncompleted_matches > 0
                  ? `${formatNumber(entry.completed_matches)} completed, ${formatNumber(entry.uncompleted_matches)} incomplete`
                  : undefined
              }>
                {formatNumber(entry.completed_matches)}
                {entry.uncompleted_matches > 0 && <sub>{formatNumber(entry.uncompleted_matches)}</sub>}
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
              <PlayerPortrait model={entry.player.model} size="sm" />
              {botsOnly && <BotBadge isBot skill={entry.player.skill || 5} />}
              {!botsOnly && <PlayerBadge playerId={entry.player.id} isVR={entry.player.is_vr} />}
              <Link to={`/players/${entry.player.id}`}>
                <ColoredText text={entry.player.is_vr ? stripVRPrefix(entry.player.name) : entry.player.name} />
              </Link>
            </td>
            <td className="stat-col sorted-col">{getAwardValue(entry)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export const BotLeaderboardPage = () => <LeaderboardPage botsOnly />;
