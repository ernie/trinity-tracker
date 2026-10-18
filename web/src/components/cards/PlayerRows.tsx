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
  model?: string;
  /** 1 red, 2 blue, 3 spectator; undefined in FFA. */
  team?: 1 | 2 | 3 | undefined;
  isBot?: boolean;
  /** Bot skill, 1–5. */
  skill?: number;
  isVR?: boolean;
  isVerified?: boolean;
  isAdmin?: boolean;
  score?: number;
  frags?: number;
  deaths?: number;
  ping?: number;
  awards?: AwardEntry[];
  /** Opens PlayerStatsModal on click. */
  playerId?: number;
  flagCarrier?: "red" | "blue" | "neutral";
  /** Harvester, live. */
  skullsCarrying?: number;
  /** Overload, live: damaging the enemy obelisk right now. */
  attackingObelisk?: boolean;
  /** Finished matches: false if the player left before the end. */
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
  highlightPlayerId?: number;
}

// The team-dot help lists every color, so it reads the same on any card.
const TEAM_DOT_HELP = `Team:
• Red or blue: the player's team in TDM, CTF, One Flag CTF, Overload and Harvester
• Green: no team, in FFA and Tournament
• Gray: left before the match ended (finished matches only)`;
export const SCORE_HELP = `Score for this match.
FFA, Tournament and TDM count frags. CTF, One Flag CTF, Overload and Harvester add points for captures, defends, returns and other objectives on top of frags.`;
export const PING_HELP = `Round-trip time to the server, in milliseconds. Bots always show 0.`;

export function PlayerRows({
  players,
  mode,
  onPlayerClick,
  highlightPlayerId,
}: PlayerRowsProps) {
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
        const highlighted =
          highlightPlayerId !== undefined && p.playerId === highlightPlayerId;
        const handleClick = clickable
          ? () => onPlayerClick!(p.name, p.cleanName ?? p.name, p.playerId)
          : undefined;
        return (
          <div
            key={i}
            className={`player-row ${mode} ${clickable ? "clickable" : ""} ${highlighted ? "highlighted" : ""}`}
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
                <PlayerPortrait model={p.model} team={p.team} size="sm" />
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
                      ? `Carrying the neutral flag. Taking it to the enemy base scores a capture.`
                      : `Carrying the ${p.flagCarrier} flag. Taking it home scores a capture.`
                  }
                >
                  {/* The runner icon (status="taken") belongs to the score indicators. */}
                  <FlagIcon team={p.flagCarrier} status="base" size="sm" />
                </span>
              )}
              {p.skullsCarrying ? (
                <span
                  className="row-carrier row-carrier--skulls"
                  aria-hidden
                  data-help={`Carrying enemy skulls. They're lost if this player dies. Taking them to the enemy obelisk scores.`}
                >
                  <SkullIcon team={p.team === 2 ? "red" : "blue"} size="sm" />
                  <span className="row-carrier__count">{p.skullsCarrying}</span>
                </span>
              ) : null}
              {p.attackingObelisk ? (
                <span
                  className="row-carrier row-carrier--obelisk-attacker"
                  data-help={`Attacking the enemy obelisk.`}
                >
                  <TargetReticleIcon
                    team={p.team === 2 ? "red" : "blue"}
                    size="sm"
                    title="Attacking the obelisk"
                  />
                </span>
              ) : null}
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
  return "free";
}
