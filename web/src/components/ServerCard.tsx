import { memo, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import type { ServerStatus, Player } from "../types";
import { FlagIcon } from "./FlagIcon";
import { SkullIcon } from "./SkullIcon";
import { CloseIcon } from "./CloseIcon";
import { useLiveData } from "../contexts/LiveDataContext";
import { useSources } from "../hooks/useSources";
import { formatGameType } from "./MatchCard";
import { useMapMeta } from "../hooks/useMapMeta";
import { RichChip } from "./cards/RichChip";
import { Scoreboard } from "./cards/Scoreboard";
import { Duelists, type DuelistData } from "./cards/Duelists";
import { classifyScores, awardsFromCounts } from "./cards/format";
import { FfaHero, useFfaHero, type FfaHeroPlayer } from "./cards/FfaHero";
import { PlayerRows, PING_HELP } from "./cards/PlayerRows";
import { SpectatorStrip } from "./cards/SpectatorStrip";

interface ServerCardProps {
  server: ServerStatus;
  isSelected?: boolean;
  /** Takes the server id so parents can pass one stable handler across all cards. */
  onSelect?: (serverId: number) => void;
  onPlayerClick?: (
    playerName: string,
    cleanName: string,
    playerId?: number,
  ) => void;
  liveness?: "live" | "stale" | "offline";
  /** Landing page Active Now shelf: the whole card is one link, to the live
   *  viewer when the server is broadcasting and the servers page otherwise. */
  landingCard?: boolean;
}

// Help text lists every state, so a reader on one learns the others.
const STATE_HELP = `Match state:
• Active: in play
• Warmup: countdown before the match
• Waiting: not enough players
• Overtime: tied at the time limit, sudden death
• Intermission: the match just ended and the next map is loading

When the server's data feed has a problem:
• Stale: the collector hasn't checked in lately
• Offline: the server can't be reached`;
const LIVE_HELP = `Click to watch this match live in your browser.
• The small number is how many people are watching (hidden when nobody is)
• The feed runs a little behind the game; hover to see the delay`;
const MAP_HELP = `The map's short name (what you'd type after /map) above its full name.`;
const LIMITS_HELP = `Score limit, if set, then match time and time limit.`;

const RED_FLAG_HELP = `Red flag:
• At base: safe at red's base
• Taken: an enemy is carrying it
• Dropped: on the ground where its carrier died; returns to base after 30 seconds`;
const BLUE_FLAG_HELP = `Blue flag:
• At base: safe at blue's base
• Taken: an enemy is carrying it
• Dropped: on the ground where its carrier died; returns to base after 30 seconds`;
const NEUTRAL_FLAG_HELP = `Neutral flag: the one flag in One Flag CTF.
• At base: in the center
• Carried: shifts toward the team that has it
• Dropped: on the ground where its carrier died; returns after 30 seconds`;
const OBELISK_HP_HELP = `Obelisk health for each team.
• Drains as the other team damages it
• Pulses while under attack
• At 0 the attackers score, and the obelisk comes back after a short delay`;
const HARVESTER_SKULLS_HELP = `Enemy skulls this team's players are carrying, added up.
Each frag makes a skull appear at the skull generator in the middle of the map. Take enemy skulls to the enemy obelisk to score. Picking up your own team's skulls keeps the enemy from getting them.`;

function isTeamGame(gameType: string): boolean {
  const teamModes = [
    "team deathmatch",
    "tdm",
    "capture the flag",
    "ctf",
    "one flag ctf",
    "1fctf",
    "overload",
    "harvester",
  ];
  return teamModes.includes(gameType.toLowerCase());
}

function isFreeForAll(gameType: string): boolean {
  const gt = gameType?.toLowerCase();
  return gt === "free for all" || gt === "ffa";
}

function isCTF(gameType: string): boolean {
  const ctfModes = ["capture the flag", "ctf", "one flag ctf", "1fctf"];
  return ctfModes.includes(gameType.toLowerCase());
}

// Flag status: 0 at base, 1 taken, 2 dropped.
function getFlagIndicator(status: number): {
  className: string;
  title: string;
  status: "base" | "taken" | "dropped";
} {
  switch (status) {
    case 0:
      return { className: "flag-base", title: "At base", status: "base" };
    case 1:
      return { className: "flag-taken", title: "Taken", status: "taken" };
    case 2:
      return { className: "flag-dropped", title: "Dropped", status: "dropped" };
    default:
      return { className: "flag-base", title: "Unknown", status: "base" };
  }
}

// One Flag CTF status: 0 at base, 2 carried by red, 3 carried by blue,
// 4 dropped.
function getNeutralFlagIndicator(status: number): {
  drift: string;
  title: string;
  status: "base" | "taken" | "dropped";
} {
  switch (status) {
    case 0:
      return {
        drift: "flag-center",
        title: "Neutral flag at base",
        status: "base",
      };
    case 2:
      return {
        drift: "flag-toward-red",
        title: "Neutral flag carried by red",
        status: "taken",
      };
    case 3:
      return {
        drift: "flag-toward-blue",
        title: "Neutral flag carried by blue",
        status: "taken",
      };
    case 4:
      return {
        drift: "flag-center",
        title: "Neutral flag dropped",
        status: "dropped",
      };
    default:
      return { drift: "flag-center", title: "Neutral flag", status: "base" };
  }
}

// The game reads g_capturelimit for every game type from CTF up
// (`g_gametype >= GT_CTF`).
function usesCaptureLimit(gameType: string): boolean {
  if (isCTF(gameType)) return true;
  const gt = gameType.toLowerCase();
  return gt === "overload" || gt === "harvester";
}

function getScoreLimit(
  gameType: string,
  serverVars?: Record<string, string>,
): number | null {
  if (!serverVars) return null;

  if (usesCaptureLimit(gameType)) {
    const limit = parseInt(serverVars.capturelimit || "0", 10);
    if (limit > 0) return limit;
  } else {
    const limit = parseInt(serverVars.fraglimit || "0", 10);
    if (limit > 0) return limit;
  }
  return null;
}

function getTimeLimit(serverVars?: Record<string, string>): number | null {
  if (!serverVars) return null;
  const limit = parseInt(serverVars.timelimit || "0", 10);
  return limit > 0 ? limit : null;
}

function formatGameTime(ms: number, isWarmup: boolean = false): string {
  const absMs = Math.abs(ms);
  const totalSeconds = Math.floor(absMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  const timeStr = `${minutes}:${seconds.toString().padStart(2, "0")}`;
  return isWarmup ? `-${timeStr}` : timeStr;
}

function getDisplayTime(
  server: ServerStatus,
  timeLimitMinutes: number | null,
): { time: string; isWarmup: boolean; isOvertime: boolean } {
  const isWarmup =
    server.match_state === "warmup" &&
    server.warmup_remaining &&
    server.warmup_remaining > 0;
  if (isWarmup) {
    return {
      time: formatGameTime(server.warmup_remaining!, true),
      isWarmup: true,
      isOvertime: false,
    };
  }
  const isOvertime = server.match_state === "overtime";
  if (isOvertime && timeLimitMinutes) {
    const timeLimitMs = timeLimitMinutes * 60 * 1000;
    const overtimeMs = server.game_time_ms - timeLimitMs;
    return {
      time: `+${formatGameTime(Math.max(0, overtimeMs))}`,
      isWarmup: false,
      isOvertime: true,
    };
  }
  return {
    time: formatGameTime(server.game_time_ms),
    isWarmup: false,
    isOvertime: false,
  };
}

// `live` stops the tick on an offline or stale server: the poller keeps
// rebroadcasting its last snapshot, so a ticking clock would jump back on
// every poll.
function useInterpolatedTime(
  server: ServerStatus,
  timeLimitMinutes: number | null,
  live: boolean,
): { gameTimeMs: number; warmupRemaining: number | undefined } {
  // Keeping the snapshot with the offset lets a new snapshot reset it during
  // render: https://react.dev/reference/react/useState#storing-information-from-previous-renders.
  const [state, setState] = useState({
    lastUpdated: server.last_updated,
    offset: 0,
  });

  if (state.lastUpdated !== server.last_updated) {
    setState({ lastUpdated: server.last_updated, offset: 0 });
  }

  useEffect(() => {
    if (!live) return;
    if (
      server.match_state !== "active" &&
      server.match_state !== "overtime" &&
      server.match_state !== "warmup"
    ) {
      return;
    }

    const interval = setInterval(() => {
      setState((s) => ({ ...s, offset: s.offset + 1000 }));
    }, 1000);

    return () => clearInterval(interval);
  }, [server.match_state, server.last_updated, live]);

  // Until the reset lands, the old snapshot's offset counts as zero.
  const offset = state.lastUpdated === server.last_updated ? state.offset : 0;

  // During warmup game_time_ms counts from map load, so once the countdown
  // ends the active clock comes from the local offset.
  let gameTimeMs: number;
  if (server.match_state === "active" || server.match_state === "overtime") {
    gameTimeMs = server.game_time_ms + offset;
  } else if (
    server.match_state === "warmup" &&
    server.warmup_remaining !== undefined
  ) {
    gameTimeMs = Math.max(0, offset - server.warmup_remaining);
  } else {
    gameTimeMs = server.game_time_ms;
  }

  // Only the server reports overtime, so the local clock stops at the limit.
  if (
    timeLimitMinutes &&
    timeLimitMinutes > 0 &&
    server.match_state === "active"
  ) {
    const timeLimitMs = timeLimitMinutes * 60 * 1000;
    if (server.game_time_ms <= timeLimitMs) {
      gameTimeMs = Math.min(gameTimeMs, timeLimitMs);
    }
  }

  const warmupRemaining =
    server.match_state === "warmup" && server.warmup_remaining !== undefined
      ? Math.max(0, server.warmup_remaining - offset)
      : server.warmup_remaining;

  return { gameTimeMs, warmupRemaining };
}

function sortPlayersByTeam(players: Player[]): Player[] {
  return [...players].sort((a, b) => {
    const teamOrder = (team?: number) => {
      switch (team) {
        case 1:
          return 0;
        case 2:
          return 1;
        case 3:
          return 2;
        default:
          return 3;
      }
    };
    const teamDiff = teamOrder(a.team) - teamOrder(b.team);
    if (teamDiff !== 0) return teamDiff;
    return b.score - a.score;
  });
}

export const MODE_PROFILES: Record<string, { icon: string; label: string }> = {
  "0": { icon: "/assets/modes/vq3.png", label: "Quake III Arena" },
  "1": { icon: "/assets/modes/cpm.png", label: "CPMA" },
  "2": { icon: "/assets/modes/ql.png", label: "Quake Live" },
  "3": { icon: "/assets/modes/qlt.png", label: "Quake Live Turbo" },
};

export function ModeIcon({ mode }: { mode?: string }) {
  const profile = MODE_PROFILES[mode ?? "0"];
  if (!profile) return null;
  return (
    <span className="mode-icons">
      <img className="mode-icon" src={profile.icon} alt={profile.label} />
      <span className="mode-panel">
        <span className="mode-panel-row">
          <img src={profile.icon} alt={profile.label} />
          <span>Mode: {profile.label}</span>
        </span>
      </span>
    </span>
  );
}

// Reading obelisk attackers out here lets the memoized ServerCardImpl skip
// context ticks that leave this card's attacker set alone.
export function ServerCard(props: ServerCardProps) {
  const { obeliskAttackersByServer } = useLiveData();
  return (
    <ServerCardImpl
      {...props}
      attackingSlots={obeliskAttackersByServer.get(props.server.server_id)}
    />
  );
}

type ServerCardImplProps = ServerCardProps & {
  attackingSlots: Set<number> | undefined;
};

const ServerCardImpl = memo(function ServerCardImpl({
  server,
  isSelected,
  onSelect,
  onPlayerClick,
  liveness,
  landingCard,
  attackingSlots,
}: ServerCardImplProps) {
  const { hasMultiple: hasMultipleSources } = useSources();
  const meta = useMapMeta(server.map);
  const showTeamScores = isTeamGame(server.game_type) && server.team_scores;
  const scoreLimit = getScoreLimit(server.game_type, server.server_vars);
  const timeLimit = getTimeLimit(server.server_vars);

  // Offline means the UDP poll is failing; stale means the collector
  // heartbeat aged out. `liveness` is undefined until /api/servers answers
  // and counts as live, so healthy cards never flash dim.
  const isDegraded =
    liveness === "offline" || liveness === "stale" || server.online === false;

  // Liveness outranks match_state: "Active" means nothing once the data is old.
  const getStateBadge = (): { className: string; label: string } => {
    if (liveness === "offline" || !server.online) {
      return { className: "state-offline", label: "Offline" };
    }
    if (liveness === "stale") {
      return { className: "state-stale", label: "Stale" };
    }
    switch (server.match_state) {
      case "overtime":
        return { className: "overtime", label: "Overtime" };
      case "warmup":
        return { className: "warmup", label: "Warmup" };
      case "waiting":
        return { className: "waiting", label: "Waiting" };
      case "intermission":
        return { className: "intermission", label: "Intermission" };
      case "active":
      default:
        return { className: "active", label: "Active" };
    }
  };
  const stateBadge = getStateBadge();

  const levelshotUrl = server.map
    ? `/assets/levelshots/${server.map.toLowerCase()}.jpg`
    : undefined;
  const cardStyle = useMemo<React.CSSProperties | undefined>(
    () =>
      levelshotUrl
        ? ({ "--levelshot": `url(${levelshotUrl})` } as React.CSSProperties)
        : undefined,
    [levelshotUrl],
  );

  const activePlayers = sortPlayersByTeam(
    (server.players ?? []).filter((p) => p.team !== 3),
  );
  const spectators = (server.players ?? []).filter((p) => p.team === 3);

  // The hook runs on every card, since hooks can't be conditional, and
  // only FFA cards show the hero. Keyed on client_num because names
  // aren't unique.
  const ffaPlayerData: FfaHeroPlayer[] = activePlayers.map((p) => ({
    key: p.client_num,
    name: p.name,
    cleanName: p.clean_name,
    model: p.model,
    isBot: p.is_bot,
    skill: p.skill,
    isVR: p.is_vr,
    isVerified: p.is_verified,
    isAdmin: p.is_admin,
    score: p.score ?? 0,
    sub: <span data-help={PING_HELP}>{p.ping ?? 0} ping</span>,
    awards: awardsFromCounts(p),
    playerId: p.player_id,
  }));
  const ffaHero = useFfaHero(ffaPlayerData);
  const isFfa = isFreeForAll(server.game_type);
  const heroKey = isFfa && ffaHero ? ffaHero.key : null;

  // The watch link follows the leader (?f=<client slot>): the FFA hero, who
  // holds through ties, or else the top score. A client slot means something
  // only while the match runs. With no leader yet, as in warmup, the viewer
  // opens at its default view.
  const liveLeader = isFfa
    ? ffaHero
      ? Number(ffaHero.key)
      : null
    : (activePlayers.reduce<Player | null>(
        (best, p) => ((p.score ?? 0) > (best?.score ?? 0) ? p : best),
        null,
      )?.client_num ?? null);
  const liveTo =
    `/tv/${server.source}/${server.key}` +
    (liveLeader != null ? `?f=${liveLeader}` : "");
  const liveViewers = server.live_viewers ?? 0;
  const watchDetails = [
    ...(liveViewers > 0 ? [`${liveViewers} watching`] : []),
    ...(server.live_delay_seconds
      ? [`delayed ~${server.live_delay_seconds}s`]
      : []),
  ];
  const watchLabel =
    watchDetails.length > 0
      ? `Watch live (${watchDetails.join(", ")})`
      : "Watch live";

  // At intermission the scoreboard shows the final result, so the cards
  // leave live mode and mark the winner. FFA gilds the leader only when
  // nobody ties the top score, as MatchCard does.
  const isIntermission = server.match_state === "intermission";
  const ffaTopCount = ffaHero
    ? ffaPlayerData.filter((p) => p.score === ffaHero.score).length
    : 0;
  const ffaGild = isIntermission && ffaTopCount === 1;

  const card = (
    <div
      className={`card server-card ${isSelected ? "selected" : ""} ${onSelect ? "selectable" : ""} ${isDegraded ? "degraded" : ""}`}
      style={cardStyle}
      onClick={onSelect ? () => onSelect(server.server_id) : undefined}
      role={onSelect ? "button" : undefined}
      tabIndex={onSelect ? 0 : undefined}
    >
      <div className="card__topbar">
        <RichChip
          source={hasMultipleSources ? server.source : undefined}
          server={server.key}
          mode={formatGameType(server.game_type)}
        >
          <ModeIcon mode={server.server_vars?.g_mode ?? "0"} />
        </RichChip>
        <div className="card__status-row">
          {server.is_live && (
            <Link
              to={liveTo}
              reloadDocument
              className="card__watch"
              onClick={(e) => e.stopPropagation()}
              aria-label={watchLabel}
              title={watchLabel}
              data-help={LIVE_HELP}
            >
              <span className="card__watch-dot" aria-hidden="true" />
              LIVE
              {liveViewers > 0 && (
                <span className="card__watch-count" aria-hidden="true">
                  {liveViewers}
                </span>
              )}
            </Link>
          )}
          <span
            className={`card__state ${stateBadge.className}`}
            data-help={STATE_HELP}
          >
            {stateBadge.label}
          </span>
        </div>
      </div>

      <div className="card__header">
        <div className="card__map" data-help={MAP_HELP}>
          {/* Both rows always render so cards line up across the grid. */}
          <span className="card__map-short">
            {meta.shortName || server.map || "Unknown"}
          </span>
          <span className="card__map-long">
            {meta.displayName || server.map || "Unknown"}
          </span>
        </div>
        <CardLimits
          server={server}
          timeLimit={timeLimit}
          scoreLimit={scoreLimit}
          live={!isDegraded}
        />
      </div>

      {showTeamScores &&
        server.team_scores &&
        (() => {
          const obj = server.obj_status;
          const redInd =
            obj?.mode === "ctf"
              ? (() => {
                  const i = getFlagIndicator(obj.red ?? 0);
                  return (
                    <span
                      className={`flag-indicator ${i.className}`}
                      data-help={RED_FLAG_HELP}
                    >
                      <FlagIcon
                        team="red"
                        status={i.status}
                        size="md"
                        title={`Red flag: ${i.title}`}
                      />
                    </span>
                  );
                })()
              : undefined;
          const blueInd =
            obj?.mode === "ctf"
              ? (() => {
                  const i = getFlagIndicator(obj.blue ?? 0);
                  return (
                    <span
                      className={`flag-indicator ${i.className}`}
                      data-help={BLUE_FLAG_HELP}
                    >
                      <FlagIcon
                        team="blue"
                        status={i.status}
                        size="md"
                        title={`Blue flag: ${i.title}`}
                      />
                    </span>
                  );
                })()
              : undefined;
          const centerInd =
            obj?.mode === "1fctf"
              ? (() => {
                  const i = getNeutralFlagIndicator(obj.neutral ?? 0);
                  return (
                    <span
                      className={`flag-indicator ${i.drift}`}
                      data-help={NEUTRAL_FLAG_HELP}
                    >
                      <FlagIcon
                        team="neutral"
                        status={i.status}
                        size="md"
                        title={i.title}
                      />
                    </span>
                  );
                })()
              : undefined;
          const maxHP =
            server.obelisk_health_max && server.obelisk_health_max > 0
              ? server.obelisk_health_max
              : 2500;
          const obeliskUnderAttack = { red: false, blue: false };
          if (attackingSlots && attackingSlots.size > 0) {
            for (const p of server.players ?? []) {
              if (!attackingSlots.has(p.client_num)) continue;
              if (p.team === 1) obeliskUnderAttack.blue = true;
              else if (p.team === 2) obeliskUnderAttack.red = true;
            }
          }
          const overloadInd = (team: "red" | "blue", hp: number) => (
            <ObeliskHPIndicator
              team={team}
              hp={hp}
              maxHP={maxHP}
              underAttack={obeliskUnderAttack[team]}
            />
          );
          // Undefined before the obelisk spawns; 0 means destroyed and
          // still draws the empty bar.
          const overloadRed =
            obj?.mode === "overload" && obj.red_obelisk_hp !== undefined
              ? overloadInd("red", obj.red_obelisk_hp)
              : undefined;
          const overloadBlue =
            obj?.mode === "overload" && obj.blue_obelisk_hp !== undefined
              ? overloadInd("blue", obj.blue_obelisk_hp)
              : undefined;
          // The icon takes the enemy's color, since a red carrier holds blue
          // skulls, as with a CTF flag carrier.
          const harvestInd = (team: "red" | "blue", skulls: number) => (
            <span
              className={`harvester-skulls harvester-skulls--${team}`}
              title={`${skulls} enemy skull${skulls === 1 ? "" : "s"} carried by ${team}`}
              data-help={HARVESTER_SKULLS_HELP}
            >
              <SkullIcon team={team === "red" ? "blue" : "red"} size="md" />
              <span className="harvester-skulls__count">{skulls}</span>
            </span>
          );
          const harvestRed =
            obj?.mode === "harvester"
              ? harvestInd("red", obj.red_skulls ?? 0)
              : undefined;
          const harvestBlue =
            obj?.mode === "harvester"
              ? harvestInd("blue", obj.blue_skulls ?? 0)
              : undefined;
          return (
            <Scoreboard
              redLabel={server.server_vars?.g_redteam ?? "Red"}
              redScore={server.team_scores.red}
              blueLabel={server.server_vars?.g_blueteam ?? "Blue"}
              blueScore={server.team_scores.blue}
              state={classifyScores(
                server.team_scores.red,
                server.team_scores.blue,
              )}
              live={!isIntermission}
              redIndicator={redInd ?? overloadRed ?? harvestRed}
              blueIndicator={blueInd ?? overloadBlue ?? harvestBlue}
              centerIndicator={centerInd}
            />
          );
        })()}

      {server.game_type === "1v1" && activePlayers.length >= 2 ? (
        <Duelists
          left={duelistFromLivePlayer(activePlayers[0])}
          right={duelistFromLivePlayer(activePlayers[1])}
          state={classifyScores(
            activePlayers[0]?.score ?? 0,
            activePlayers[1]?.score ?? 0,
          )}
          live={!isIntermission}
          onPlayerClick={onPlayerClick}
        />
      ) : (
        <>
          {isFfa && (
            <FfaHero
              player={ffaHero}
              gildScore={ffaGild}
              onPlayerClick={onPlayerClick}
            />
          )}
          <PlayerRows
            players={activePlayers
              .filter((p) => !(heroKey != null && p.client_num === heroKey))
              .map((p) => ({
                name: p.name,
                cleanName: p.clean_name,
                model: p.model,
                team: p.team as 1 | 2 | undefined,
                isBot: p.is_bot,
                skill: p.skill,
                isVR: p.is_vr,
                isVerified: p.is_verified,
                isAdmin: p.is_admin,
                score: p.score,
                ping: p.ping,
                awards: awardsFromCounts(p),
                playerId: p.player_id,
                flagCarrier: flagCarrierForClient(
                  server.obj_status,
                  p.client_num,
                ),
                skullsCarrying: p.skulls_carrying,
                attackingObelisk: attackingSlots?.has(p.client_num) ?? false,
              }))}
            mode="live"
            onPlayerClick={onPlayerClick}
          />
        </>
      )}

      <SpectatorStrip
        spectators={spectators.map((p) => ({ name: p.name, isVR: p.is_vr }))}
        isLive={true}
      />
    </div>
  );

  // .landing-watch-link turns off pointer events inside the card, so the
  // whole card is one link.
  if (landingCard) {
    return server.is_live ? (
      <Link
        to={liveTo}
        reloadDocument
        className="landing-watch-link"
        aria-label={watchLabel}
      >
        {card}
      </Link>
    ) : (
      <Link
        to="/servers"
        className="landing-watch-link"
        aria-label="Browse servers"
      >
        {card}
      </Link>
    );
  }
  return card;
});

// Holds the one-second tick so only this subtree re-renders.
const CardLimits = memo(function CardLimits({
  server,
  timeLimit,
  scoreLimit,
  live,
}: {
  server: ServerStatus;
  timeLimit: number | null;
  scoreLimit: number | null;
  live: boolean;
}) {
  const { gameTimeMs, warmupRemaining } = useInterpolatedTime(
    server,
    timeLimit,
    live,
  );
  const interpolatedServer = {
    ...server,
    game_time_ms: gameTimeMs,
    warmup_remaining: warmupRemaining,
  };
  const displayTime = getDisplayTime(interpolatedServer, timeLimit);

  if (!scoreLimit && !timeLimit && gameTimeMs <= 0 && !displayTime.isWarmup)
    return null;

  return (
    <div className="card__limits" data-help={LIMITS_HELP}>
      {scoreLimit && <span>Limit: {scoreLimit}</span>}
      {scoreLimit && (timeLimit || gameTimeMs > 0 || displayTime.isWarmup) && (
        <span> | </span>
      )}
      <span
        className={
          displayTime.isOvertime
            ? "overtime-time"
            : displayTime.isWarmup
              ? "warmup-time"
              : ""
        }
      >
        {displayTime.time}
      </span>
      {timeLimit && <span> / {timeLimit}m</span>}
    </div>
  );
});

function duelistFromLivePlayer(p: Player | undefined): DuelistData {
  if (!p) {
    return { name: "—", score: 0 };
  }
  return {
    name: p.name,
    cleanName: p.clean_name,
    model: p.model,
    isBot: p.is_bot,
    skill: p.skill,
    isVR: p.is_vr,
    isVerified: p.is_verified,
    isAdmin: p.is_admin,
    score: p.score ?? 0,
    sub: <span data-help={PING_HELP}>{p.ping ?? 0} ping</span>,
    awards: awardsFromCounts(p),
    playerId: p.player_id,
  };
}

// At 0 HP the bar stays, empty and pulsing, while the obelisk respawns.
function ObeliskHPIndicator({
  team,
  hp,
  maxHP,
  underAttack,
}: {
  team: "red" | "blue";
  hp: number;
  maxHP: number;
  underAttack?: boolean;
}) {
  const pct = Math.max(0, Math.min(100, (hp / maxHP) * 100));
  const destroyed = hp <= 0;
  const attacked = !!underAttack && !destroyed;
  const label = destroyed
    ? `${team} obelisk destroyed, respawning…`
    : attacked
      ? `${team} obelisk under attack: ${hp} / ${maxHP} HP`
      : `${team} obelisk: ${hp} / ${maxHP} HP`;
  return (
    <span
      className={`obelisk-hp obelisk-hp--${team}${destroyed ? " obelisk-hp--destroyed" : ""}${attacked ? " obelisk-hp--under-attack" : ""}`}
      title={label}
      role="img"
      aria-label={label}
      data-help={OBELISK_HP_HELP}
    >
      <span className="obelisk-hp__bar">
        <span className="obelisk-hp__fill" style={{ height: `${pct}%` }} />
      </span>
      <span className="obelisk-hp__pct" aria-hidden="true">
        {destroyed ? <CloseIcon size={9} /> : `${Math.round(pct)}%`}
      </span>
    </span>
  );
}

function flagCarrierForClient(
  obj: import("../types").ObjStatus | undefined,
  clientNum: number,
): "red" | "blue" | "neutral" | undefined {
  if (!obj) return undefined;
  if (obj.mode === "ctf") {
    if (obj.red_carrier === clientNum && obj.red === 1) return "red";
    if (obj.blue_carrier === clientNum && obj.blue === 1) return "blue";
  }
  if (obj.mode === "1fctf" && obj.neutral_carrier === clientNum) {
    return "neutral";
  }
  return undefined;
}
