export type ScoreState = "left" | "right" | "tie" | "no_contest";

const EXIT_REASONS: Record<string, string> = {
  score_limit: "Score limit reached",
  time_limit: "Time limit",
  overtime: "Decided in OT",
  sudden_death: "Decided in OT",
  intermission: "Match ended",
  forfeit: "Forfeit",
  tie: "Tie",
  no_contest: "No contest",
  client_disconnect: "Client disconnect",
};

export function formatExitReason(reason?: string | null): string {
  if (!reason) return "";
  if (EXIT_REASONS[reason]) return EXIT_REASONS[reason];
  return reason.charAt(0).toUpperCase() + reason.slice(1).replace(/_/g, " ");
}

export function classifyScores(left: number, right: number): ScoreState {
  if (left <= 0 && right <= 0) return "no_contest";
  if (left === right) return "tie";
  return left > right ? "left" : "right";
}

export type MedalType =
  | "impressive"
  | "excellent"
  | "humiliation"
  | "capture"
  | "assist"
  | "defend"
  | "flag_return"
  | "victory"
  | "skull"
  | "obelisk";

export interface AwardEntry {
  type: MedalType;
  count: number;
  /** flag_return only: which team's flag to draw. */
  team?: "red" | "blue" | "neutral";
}

export interface PlayerAwardCounts {
  impressives?: number;
  excellents?: number;
  humiliations?: number;
  captures?: number;
  assists?: number;
  defends?: number;
  flag_returns?: number;
  victories?: number;
  skulls_delivered?: number;
  obelisks_destroyed?: number;
  /** 1 red, 2 blue; colors the flag_return medal. */
  team?: number;
}

/** Medals run from least to most tied to the score, ending in victory,
 *  so in row layouts the medal nearest the score column is the one most
 *  responsible for it. */
export function awardsFromCounts(p: PlayerAwardCounts): AwardEntry[] {
  const order: Array<[MedalType, number | undefined]> = [
    ["humiliation", p.humiliations],
    ["impressive", p.impressives],
    ["excellent", p.excellents],
    ["assist", p.assists],
    ["defend", p.defends],
    ["flag_return", p.flag_returns],
    ["capture", p.captures],
    ["skull", p.skulls_delivered],
    ["obelisk", p.obelisks_destroyed],
    ["victory", p.victories],
  ];
  const flagTeam: AwardEntry["team"] =
    p.team === 1 ? "red" : p.team === 2 ? "blue" : "neutral";
  return order
    .filter(
      (entry): entry is [MedalType, number] =>
        typeof entry[1] === "number" && entry[1] > 0,
    )
    .map(([type, count]) =>
      type === "flag_return"
        ? { type, count, team: flagTeam }
        : { type, count },
    );
}
