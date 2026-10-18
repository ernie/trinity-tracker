import type { TimePeriod } from "../types";

// The periods are rolling windows, so "Day" means the past 24 hours.
export const PERIOD_LABELS: Record<TimePeriod, string> = {
  all: "All time",
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
