import type { TimePeriod } from "../types";

// Tight chip labels for the segmented period control. These name the
// rolling window, not a calendar boundary — the backend computes
// "asOf - 24h / -7d / -30d / -365d" (storage/sqlite.go getTimePeriodBounds),
// so "Day" means past day, never "since midnight". The banner tagline
// spells that out explicitly ("PAST 24 HOURS" etc.) via PERIOD_DISPLAY.
export const PERIOD_LABELS: Record<TimePeriod, string> = {
  all: "All Time",
  day: "Day",
  week: "Week",
  month: "Month",
  year: "Year",
};

export type GameTypeFilter =
  | "all"
  | "ffa"
  | "tdm"
  | "ctf"
  | "1fctf"
  | "1v1"
  | "overload"
  | "harvester";

// Filter-button order; 'all' is rendered separately. Labels via formatGameType().
export const GAME_TYPES: readonly Exclude<GameTypeFilter, "all">[] = [
  "1fctf",
  "1v1",
  "ctf",
  "ffa",
  "harvester",
  "overload",
  "tdm",
];

export function isGameTypeFilter(s: string): s is GameTypeFilter {
  return s === "all" || (GAME_TYPES as readonly string[]).includes(s);
}
