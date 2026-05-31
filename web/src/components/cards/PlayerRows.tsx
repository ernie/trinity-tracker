// Player table for team / FFA cards. Live shows ping; finished shows
// F+D+Score. Awards collapse into one icon per type with a count
// badge. CTF/1FCTF carrier renders to the right of the name.
import { ColoredText } from "../ColoredText";
import { PlayerPortrait } from "../PlayerPortrait";
import { PlayerBadge } from "../PlayerBadge";
import { BotBadge } from "../BotBadge";
import { MedalIcon } from "../MedalIcon";
import { FlagIcon } from "../FlagIcon";
import { SkullIcon } from "../SkullIcon";
import { TargetReticleIcon } from "../TargetReticleIcon";
import type { AwardEntry } from "./format";
import { stripVRPrefix } from "../../utils";

export interface PlayerRowData {
  name: string;
  cleanName?: string;
  /** Q3 model id used by PlayerPortrait. */
  model?: string;
  team?: 1 | 2 | 3 | undefined; // 1=red, 2=blue, 3=spec, undef=free
  isBot?: boolean;
  /** Bot skill 1–5; drives BotBadge color. Ignored for humans. */
  skill?: number;
  isVR?: boolean;
  isVerified?: boolean;
  isAdmin?: boolean;
  /** "score" for live; undefined for finished (use frags instead) */
  score?: number;
  frags?: number;
  deaths?: number;
  ping?: number;
  awards?: AwardEntry[];
  /** DB player ID — opens PlayerStatsModal on click. */
  playerId?: number;
  /** Active CTF / 1FCTF flag this player is carrying, if any. */
  flagCarrier?: "red" | "blue" | "neutral";
  /** Harvester live: enemy skulls this player is currently carrying. */
  skullsCarrying?: number;
  /** Overload live: true while actively damaging the enemy obelisk. */
  attackingObelisk?: boolean;
  /** Finished matches: false ⇒ dropped before the match ended,
   *  rendered with a gray team-dot regardless of team. */
  completed?: boolean;
}

type Mode = "live" | "finished";

interface PlayerRowsProps {
  players: PlayerRowData[];
  mode: Mode;
  onPlayerClick?: (
    playerName: string,
    cleanName: string,
    playerId?: number,
  ) => void;
}

// HelpMode descriptions for the inline cells. The team-dot text
// describes ALL color variants (red, blue, green, gray) so the same
// string works across modes — a reader on an FFA card learns about
// red/blue too, and vice versa.
const TEAM_DOT_HELP = `Team marker.
• Red / Blue — team in TDM, CTF, Overload, Harvester
• Green — no team (FFA, Tournament)
• Gray — player dropped before the match ended (completed matches only)`;
export const SCORE_HELP = `Score for this match.
FFA / 1v1 / TDM use frags; objective modes (CTF, 1FCTF, Overload, Harvester) use the engine's composite score (caps, defends, returns, frags, plus mode-specific events).`;
export const PING_HELP = `Round-trip latency to the server, in milliseconds. Bots are always 0.`;

export function PlayerRows({ players, mode, onPlayerClick }: PlayerRowsProps) {
  return (
    <div className="player-rows">
      <div className={`player-row header ${mode}`}>
        <span>Player</span>
        <span style={{ textAlign: "right" }}>
          {mode === "live" ? "Score" : "K"}
        </span>
        {mode === "finished" && <span style={{ textAlign: "right" }}>D</span>}
        {mode === "finished" && (
          <span style={{ textAlign: "right" }}>Score</span>
        )}
        {mode === "live" && <span style={{ textAlign: "right" }}>Ping</span>}
      </div>
      {players.map((p, i) => {
        const clickable = !!onPlayerClick && !p.isBot;
        const handleClick = clickable
          ? () => onPlayerClick!(p.name, p.cleanName ?? p.name, p.playerId)
          : undefined;
        return (
          <div
            key={i}
            className={`player-row ${mode} ${clickable ? "clickable" : ""}`}
            onClick={handleClick}
            role={clickable ? "button" : undefined}
            tabIndex={clickable ? 0 : undefined}
            onKeyDown={
              clickable
                ? (e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      handleClick!();
                    }
                  }
                : undefined
            }
          >
            <span className="pname-cell">
              <span
                className={`team-dot ${p.completed === false ? "dropped" : teamClass(p.team)}`}
                aria-hidden
                title={
                  p.completed === false ? "Didn't finish the match" : undefined
                }
                data-help={TEAM_DOT_HELP}
              />
              <span className="player-row__portrait">
                <PlayerPortrait model={p.model} size="sm" />
              </span>
              {p.isBot ? (
                <BotBadge isBot skill={p.skill ?? 1} size="sm" />
              ) : (
                <PlayerBadge
                  isVerified={p.isVerified}
                  isAdmin={p.isAdmin}
                  isVR={p.isVR}
                  size="sm"
                />
              )}
              <span className={`name ${p.isBot ? "bot" : ""}`}>
                <ColoredText text={p.isVR ? stripVRPrefix(p.name) : p.name} />
              </span>
              {p.flagCarrier && (
                <span
                  className="row-carrier"
                  aria-hidden
                  data-help={
                    p.flagCarrier === "neutral"
                      ? `Carrying the neutral flag toward the enemy base for a capture.`
                      : `Carrying the ${p.flagCarrier} team's flag toward their own base for a capture.`
                  }
                >
                  {/* Static flag silhouette next to the name — the
                      runner icon (status="taken") is reserved for
                      score-cell indicators. */}
                  <FlagIcon team={p.flagCarrier} status="base" size="sm" />
                </span>
              )}
              {/* Harvester live carry — opposing-team skull (CTF flag-carrier convention). */}
              {p.skullsCarrying ? (
                <span
                  className="row-carrier row-carrier--skulls"
                  aria-hidden
                  data-help={`Currently carrying enemy skulls (up to 5). On death, the skulls are removed from play. Deliver to the enemy receptacle to score.`}
                >
                  <SkullIcon team={p.team === 2 ? "red" : "blue"} size="sm" />
                  <span className="row-carrier__count">{p.skullsCarrying}</span>
                </span>
              ) : null}
              {/* Overload live attack — reticle in the targeted obelisk's color. */}
              {p.attackingObelisk ? (
                <span
                  className="row-carrier row-carrier--obelisk-attacker"
                  data-help={`Currently attacking the enemy obelisk.`}
                >
                  <TargetReticleIcon
                    team={p.team === 2 ? "red" : "blue"}
                    size="sm"
                    title="Attacking obelisk"
                  />
                </span>
              ) : null}
              {/* Cumulative awards (captures, skulls/obelisks delivered,
                  impressives, etc.) — never row-carrier, always medal-strip. */}
              {p.awards && p.awards.length > 0 && (
                <span className="row-awards">
                  {p.awards.map((a) => (
                    <MedalIcon
                      key={a.type}
                      type={a.type}
                      count={a.count}
                      size="sm"
                      team={a.team}
                    />
                  ))}
                </span>
              )}
            </span>
            <span className="stat" data-help={SCORE_HELP}>
              {mode === "live" ? (p.score ?? 0) : (p.frags ?? 0)}
            </span>
            {mode === "finished" && (
              <span className="stat dim">{p.deaths ?? 0}</span>
            )}
            {mode === "finished" && (
              <span className="stat">{p.score ?? p.frags ?? 0}</span>
            )}
            {mode === "live" && (
              <span className="stat dim" data-help={PING_HELP}>
                {p.ping ?? 0}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}

function teamClass(t?: number) {
  if (t === 1) return "red";
  if (t === 2) return "blue";
  if (t === 3) return "spec";
  // Free / undefined team — covers FFA, where the player has no team
  // affiliation but was actively in the match. Distinct from .dropped.
  return "free";
}
