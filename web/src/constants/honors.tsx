import type { ReactNode } from "react";
import { FlagPair } from "../components/FlagPair";
import type { PlayerStatsResponse } from "../types";

type StatsNumbers = PlayerStatsResponse["stats"];

// The featured honor sits beside the portrait in PlayerHero and the other
// nine fill HonorsPanel's 3x3 grid. The order makes three rows: victory and
// combat medals, objective scores, then defense and support.
//
// `key` is saved as users.featured_honor and checked against the list in
// internal/api/featured_honor.go; a new key goes in both places.
export interface HonorEntry {
  key: string;
  label: string;
  icon: string;
  /** A composed emblem, such as the flag pair for flag_returns, that
   *  HonorsPanel and PlayerHero draw in place of `icon`. */
  iconElement?: ReactNode;
  value: (s: StatsNumbers) => number;
}

export const HONORS: readonly HonorEntry[] = [
  {
    key: "victories",
    label: "Victory",
    icon: "/assets/medals/medal_victory.png",
    value: (s) => s.victories,
  },
  {
    key: "excellents",
    label: "Excellent",
    icon: "/assets/medals/medal_excellent.png",
    value: (s) => s.excellents,
  },
  {
    key: "impressives",
    label: "Impressive",
    icon: "/assets/medals/medal_impressive.png",
    value: (s) => s.impressives,
  },
  {
    key: "humiliations",
    label: "Humiliation",
    icon: "/assets/medals/medal_gauntlet.png",
    value: (s) => s.humiliations,
  },
  {
    key: "captures",
    label: "Captures",
    icon: "/assets/medals/medal_capture.png",
    value: (s) => s.captures,
  },
  {
    key: "skulls_delivered",
    label: "Skulls",
    icon: "/assets/medals/medal_skull.png",
    value: (s) => s.skulls_delivered,
  },
  {
    key: "obelisks_destroyed",
    label: "Obelisks",
    icon: "/assets/medals/medal_obelisk.png",
    value: (s) => s.obelisks_destroyed,
  },
  {
    key: "flag_returns",
    label: "Returns",
    icon: "/assets/flags/flag_in_base_red.png",
    iconElement: <FlagPair />,
    value: (s) => s.flag_returns,
  },
  {
    key: "assists",
    label: "Assists",
    icon: "/assets/medals/medal_assist.png",
    value: (s) => s.assists,
  },
  {
    key: "defends",
    label: "Defense",
    icon: "/assets/medals/medal_defend.png",
    value: (s) => s.defends,
  },
] as const;

/** Featured when the player's user hasn't picked one, or there's no user. */
export const DEFAULT_FEATURED_HONOR_KEY = "victories";
