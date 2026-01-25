import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { Link } from "react-router-dom";
import { useWebSocket } from "./useWebSocket";
import { useAuth } from "./hooks/useAuth";
import {
  ServerCard,
  ActivityLog,
  ConnectionStatus,
  RecentMatches,
  LoginForm,
  RconSidebar,
  PlayerStatsModal,
  PageNav,
  AppLogo,
} from "./components";
import { PasswordChangeModal } from "./components/PasswordChangeModal";
import { UserManagement } from "./components/UserManagement";
import type {
  Server,
  ServerStatus,
  ActivityItem,
  ActivityPlayer,
  ActivityType,
  PlayerJoinData,
  PlayerLeaveData,
  MatchStartData,
  WSEvent,
  FlagCaptureData,
  FlagTakenData,
  FlagReturnData,
  FlagDropData,
  ObeliskDestroyData,
  SkullScoreData,
  TeamChangeData,
  SayData,
  SayTeamData,
  TellData,
  SayRconData,
  AwardData,
} from "./types";

// Q3 color reset code
const COLOR_RESET = "^7";

// Strip Q3 color codes from a name
function cleanQ3Name(name: string): string {
  return name.replace(/\^[0-9]/g, "");
}

function App() {
  const [servers, setServers] = useState<Map<number, ServerStatus>>(new Map());
  const [activities, setActivities] = useState<ActivityItem[]>([]);
  const [newPlayers, setNewPlayers] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [selectedServerId, setSelectedServerId] = useState<number | null>(null);
  const [showRcon, setShowRcon] = useState(false);
  const [selectedPlayer, setSelectedPlayer] = useState<{
    name: string;
    playerId: number;
  } | null>(null);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(true);
  const [showPasswordChange, setShowPasswordChange] = useState(false);
  const [showUserManagement, setShowUserManagement] = useState(false);
  const activityIdRef = useRef(0);
  const serversRef = useRef<Map<number, ServerStatus>>(new Map());

  const { auth, login, logout, changePassword } = useAuth();

  // Show password change modal if required
  useEffect(() => {
    if (auth.isAuthenticated && auth.passwordChangeRequired) {
      setShowPasswordChange(true);
    }
  }, [auth.isAuthenticated, auth.passwordChangeRequired]);

  // Keep ref in sync with state
  serversRef.current = servers;

  // Count active human players across all servers (for sidebar indicator)
  const activeHumanPlayersCount = useMemo(() => {
    let count = 0;
    for (const [, status] of servers.entries()) {
      if (!status.players || !status.online) continue;
      const serverName = status.name && !/^Server \d+$/.test(status.name) ? status.name : null;
      if (!serverName) continue;
      for (const player of status.players) {
        if (!player.is_bot && player.team !== 3) {
          count++;
        }
      }
    }
    return count;
  }, [servers]);

  // Get server name by ID - returns undefined if real name not yet available
  const getServerName = useCallback((serverId: number): string | undefined => {
    const server = serversRef.current.get(serverId);
    const name = server?.name;
    // Don't return placeholder names like "Server 4"
    if (!name || /^Server \d+$/.test(name)) {
      return undefined;
    }
    return name;
  }, []);

  // Get bot info (isBot and skill) by looking up player in server status
  const getPlayerBotInfo = useCallback(
    (
      serverId: number,
      cleanName: string,
    ): { isBot: boolean; skill?: number } => {
      const server = serversRef.current.get(serverId);
      if (!server?.players) return { isBot: false };
      const player = server.players.find((p) => p.clean_name === cleanName);
      return { isBot: player?.is_bot ?? false, skill: player?.skill };
    },
    [],
  );

  // Determine WebSocket URL based on current location
  const wsUrl = `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/ws`;

  // Add activity helper with extra fields for filtering and icons
  const addActivity = useCallback(
    (
      type: ActivityItem["type"],
      message: string,
      options?: {
        player?: ActivityPlayer
        serverId?: number
        serverName?: string
        activityType?: ActivityType
        team?: number
        awardType?: ActivityItem["awardType"]
        mapName?: string
        victim?: ActivityPlayer
      },
    ) => {
      setActivities((prev) => {
        const newActivity: ActivityItem = {
          id: ++activityIdRef.current,
          timestamp: new Date(),
          type,
          message,
          player: options?.player,
          serverId: options?.serverId,
          serverName: options?.serverName,
          activityType: options?.activityType,
          team: options?.team,
          awardType: options?.awardType,
          mapName: options?.mapName,
          victim: options?.victim,
        };
        return [newActivity, ...prev].slice(0, 50); // Keep last 50
      });
    },
    [],
  );

  // Handle WebSocket events via callback (handles all events, including batched ones)
  const handleEvent = useCallback(
    (event: WSEvent) => {
      switch (event.event) {
        case "server_update": {
          const status = event.data as ServerStatus;
          setServers((prev) => new Map(prev).set(event.server_id, status));
          break;
        }

        case "player_join": {
          const data = event.data as PlayerJoinData;
          // Nobody cares when a bot joins :`(
          if (data.player.is_bot) break;

          const serverName = getServerName(event.server_id);
          const player = {
            name: data.player.name,
            cleanName: data.player.clean_name,
            playerId: data.player_id,
            isBot: data.player.is_bot,
            skill: data.player.skill,
          };
          addActivity("join", `${data.player.name}${COLOR_RESET} joined`, {
            player,
            serverId: event.server_id,
            serverName,
          });

          // Mark human players as new for 60 seconds (for visual highlight)
          if (!data.player.is_bot) {
            setNewPlayers((prev) => new Set(prev).add(data.player.clean_name));
            setTimeout(() => {
              setNewPlayers((prev) => {
                const next = new Set(prev);
                next.delete(data.player.clean_name);
                return next;
              });
            }, 60000);
          }
          break;
        }

        case "player_leave": {
          const data = event.data as PlayerLeaveData;
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          // Nobody cares when a bot leaves :`(
          if (botInfo.isBot) break;

          const serverName = getServerName(event.server_id);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("leave", `${data.player_name}${COLOR_RESET} left`, {
            player,
            serverId: event.server_id,
            serverName,
          });
          break;
        }

        case "match_start": {
          const data = event.data as MatchStartData;
          const serverName = getServerName(event.server_id);
          addActivity("info", `New map: ${data.map}`, {
            serverId: event.server_id,
            serverName,
            activityType: "match_start",
            mapName: data.map,
          });
          break;
        }

        case "flag_capture": {
          const data = event.data as FlagCaptureData;
          const serverName = getServerName(event.server_id);
          const flagTeam = data.team === 1 ? "Blue" : "Red"; // Captured enemy flag
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("info", `${data.player_name}${COLOR_RESET} captured the ${flagTeam} flag!`, {
            player,
            serverId: event.server_id,
            serverName,
            activityType: "flag_capture",
            team: data.team,
          });
          break;
        }

        case "flag_taken": {
          const data = event.data as FlagTakenData;
          const serverName = getServerName(event.server_id);
          const flagTeam = data.team === 1 ? "Red" : "Blue";
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("info", `${data.player_name}${COLOR_RESET} took the ${flagTeam} flag`, {
            player,
            serverId: event.server_id,
            serverName,
            activityType: "flag_taken",
            team: data.team,
          });
          break;
        }

        case "flag_return": {
          const data = event.data as FlagReturnData;
          const serverName = getServerName(event.server_id);
          const flagTeam = data.team === 1 ? "Red" : "Blue";
          if (data.player_name) {
            const cleanName = cleanQ3Name(data.player_name);
            const botInfo = getPlayerBotInfo(event.server_id, cleanName);
            const player = {
              name: data.player_name,
              cleanName,
              playerId: data.player_id,
              ...botInfo,
            };
            addActivity("info", `${data.player_name}${COLOR_RESET} returned the ${flagTeam} flag`, {
              player,
              serverId: event.server_id,
              serverName,
              activityType: "flag_return",
              team: data.team,
            });
          } else {
            addActivity("info", `The ${flagTeam} flag was returned`, {
              serverId: event.server_id,
              serverName,
              activityType: "flag_return",
              team: data.team,
            });
          }
          break;
        }

        case "flag_drop": {
          const data = event.data as FlagDropData;
          const serverName = getServerName(event.server_id);
          const flagTeam = data.team === 1 ? "Red" : "Blue";
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("info", `${data.player_name}${COLOR_RESET} dropped the ${flagTeam} flag`, {
            player,
            serverId: event.server_id,
            serverName,
            activityType: "flag_drop",
            team: data.team,
          });
          break;
        }

        case "obelisk_destroy": {
          const data = event.data as ObeliskDestroyData;
          const serverName = getServerName(event.server_id);
          const teamName = data.team === 1 ? "Red" : "Blue";
          const cleanName = cleanQ3Name(data.attacker_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.attacker_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("info", `${data.attacker_name}${COLOR_RESET} destroyed the ${teamName} obelisk!`, {
            player,
            serverId: event.server_id,
            serverName,
            activityType: "obelisk_destroy",
            team: data.team,
          });
          break;
        }

        case "skull_score": {
          const data = event.data as SkullScoreData;
          const serverName = getServerName(event.server_id);
          const teamName = data.team === 1 ? "Red" : "Blue";
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("info", `${data.player_name}${COLOR_RESET} scored ${data.skulls} skull${data.skulls !== 1 ? "s" : ""} for ${teamName}`, {
            player,
            serverId: event.server_id,
            serverName,
            activityType: "skull_score",
            team: data.team,
          });
          break;
        }

        case "team_change": {
          const data = event.data as TeamChangeData;
          const serverName = getServerName(event.server_id);
          const teamNames = ["Free", "Red", "Blue", "Spectator"];
          const oldTeam = teamNames[data.old_team] || "Unknown";
          const newTeam = teamNames[data.new_team] || "Unknown";
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("info", `${data.player_name}${COLOR_RESET}: ${oldTeam} → ${newTeam}`, {
            player,
            serverId: event.server_id,
            serverName,
            activityType: "team_change",
          });
          break;
        }

        case "say": {
          const data = event.data as SayData;
          const serverName = getServerName(event.server_id);
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("chat", `${data.player_name}${COLOR_RESET}: ${data.message}`, {
            player,
            serverId: event.server_id,
            serverName,
          });
          break;
        }

        case "say_team": {
          const data = event.data as SayTeamData;
          const serverName = getServerName(event.server_id);
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };
          addActivity("chat", `(team) ${data.player_name}${COLOR_RESET}: ${data.message}`, {
            player,
            serverId: event.server_id,
            serverName,
          });
          break;
        }

        case "tell": {
          const data = event.data as TellData;
          const serverName = getServerName(event.server_id);
          // For tell, use the sender as the clickable player
          const cleanName = cleanQ3Name(data.from_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.from_name,
            cleanName,
            playerId: data.from_player_id,
            ...botInfo,
          };
          addActivity("chat", `${data.from_name}${COLOR_RESET} -> ${data.to_name}${COLOR_RESET}: ${data.message}`, {
            player,
            serverId: event.server_id,
            serverName,
          });
          break;
        }

        case "say_rcon": {
          const data = event.data as SayRconData;
          const serverName = getServerName(event.server_id);
          addActivity("chat", `server: ${data.message}`, {
            serverId: event.server_id,
            serverName,
          });
          break;
        }

        case "award": {
          const data = event.data as AwardData;
          const serverName = getServerName(event.server_id);
          const cleanName = cleanQ3Name(data.player_name);
          const botInfo = getPlayerBotInfo(event.server_id, cleanName);
          const player = {
            name: data.player_name,
            cleanName,
            playerId: data.player_id,
            ...botInfo,
          };

          // Build award message based on type
          let message: string;
          let victim: ActivityPlayer | undefined;
          switch (data.award_type) {
            case "impressive":
              message = `${data.player_name}${COLOR_RESET} was impressive!`;
              break;
            case "excellent":
              message = `${data.player_name}${COLOR_RESET} was excellent!`;
              break;
            case "humiliation":
              if (data.victim_name) {
                const victimCleanName = cleanQ3Name(data.victim_name);
                const victimBotInfo = getPlayerBotInfo(event.server_id, victimCleanName);
                victim = {
                  name: data.victim_name,
                  cleanName: victimCleanName,
                  playerId: data.victim_player_id,
                  ...victimBotInfo,
                };
                message = `${data.player_name}${COLOR_RESET} humiliated ${data.victim_name}${COLOR_RESET}!`;
              } else {
                message = `${data.player_name}${COLOR_RESET} earned a humiliation!`;
              }
              break;
            case "defend": {
              const flagTeam = data.team === 1 ? "Red" : data.team === 2 ? "Blue" : null;
              if (flagTeam) {
                message = `${data.player_name}${COLOR_RESET} defended the ${flagTeam} flag!`;
              } else {
                message = `${data.player_name}${COLOR_RESET} defended the flag!`;
              }
              break;
            }
            case "assist":
              message = `${data.player_name}${COLOR_RESET} assisted a capture!`;
              break;
            default:
              message = `${data.player_name}${COLOR_RESET} earned ${data.award_type}!`;
          }

          addActivity("info", message, {
            player,
            serverId: event.server_id,
            serverName,
            activityType: "award",
            awardType: data.award_type,
            victim,
          });
          break;
        }
      }
    },
    [addActivity, getServerName, getPlayerBotInfo],
  );

  const { isConnected } = useWebSocket(wsUrl, handleEvent);

  // Fetch initial server data
  useEffect(() => {
    async function fetchServers() {
      try {
        const res = await fetch("/api/servers");
        const serverList: Server[] = await res.json();

        const statusMap = new Map<number, ServerStatus>();

        await Promise.all(
          serverList.map(async (server) => {
            try {
              const statusRes = await fetch(`/api/servers/${server.id}/status`);
              if (statusRes.ok) {
                const status: ServerStatus = await statusRes.json();
                statusMap.set(server.id, status);
              }
            } catch (e) {
              console.error(
                `Failed to fetch status for server ${server.id}:`,
                e,
              );
            }
          }),
        );

        setServers(statusMap);
      } catch (e) {
        console.error("Failed to fetch servers:", e);
      } finally {
        setLoading(false);
      }
    }

    fetchServers();
  }, []);

  // Handle server selection
  const handleServerSelect = (serverId: number) => {
    setSelectedServerId(serverId);
    if (auth.isAuthenticated) {
      setShowRcon(true);
    }
  };

  // Handle player click to show stats modal (only if playerId is available)
  const handlePlayerClick = (
    playerName: string,
    _cleanName: string,
    playerId?: number,
  ) => {
    if (playerId) {
      setSelectedPlayer({ name: playerName, playerId });
    }
  };

  const selectedServer =
    selectedServerId !== null ? servers.get(selectedServerId) || null : null;

  if (loading) {
    return (
      <div className="app">
        <h1>
          <AppLogo linkToHome={false} />
          Trinity
        </h1>
        <div className="loading">Loading servers...</div>
      </div>
    );
  }

  const serverList = Array.from(servers.values()).sort(
    (a, b) => a.server_id - b.server_id,
  );

  return (
    <div
      className={`app ${showRcon && auth.isAuthenticated ? "with-right-sidebar" : ""} ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}
    >
      <ConnectionStatus isConnected={isConnected} />

      <header className="app-header">
        <h1>
          <AppLogo linkToHome={false} />
          Trinity
        </h1>
        <PageNav />
        <div className="auth-section">
          {auth.isAuthenticated ? (
            <div className="user-info">
              <Link to="/account" className="username-link">
                {auth.username}
              </Link>
              {auth.isAdmin && (
                <button
                  onClick={() => setShowUserManagement(true)}
                  className="admin-btn"
                >
                  Users
                </button>
              )}
              <button onClick={logout} className="logout-btn">
                Logout
              </button>
            </div>
          ) : (
            <LoginForm
              onLogin={(username, password) => login({ username, password })}
            />
          )}
        </div>
      </header>

      <aside className={`left-sidebar ${sidebarCollapsed ? "collapsed" : ""}`}>
        <div className="sidebar-header">
          {sidebarCollapsed ? (
            <div className="collapsed-header">
              <div
                className="activity-indicator-wrapper"
                title={activeHumanPlayersCount > 0 ? `Players online: ${activeHumanPlayersCount}` : "No players online"}
              >
                <span className={`activity-indicator ${activeHumanPlayersCount > 0 ? "active" : "inactive"}`} />
                {activeHumanPlayersCount > 0 && (
                  <span className="activity-count">{activeHumanPlayersCount}</span>
                )}
              </div>
              <span className="rotated-title">Activity</span>
            </div>
          ) : (
            <h2>Activity</h2>
          )}
          <button
            className="collapse-toggle"
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            {sidebarCollapsed ? "»" : "«"}
          </button>
        </div>
        {!sidebarCollapsed && (
          <ActivityLog
            activities={activities}
            servers={servers}
            onPlayerClick={handlePlayerClick}
          />
        )}
      </aside>

      <div
        className={`app-layout ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}
      >
        <div className="main-content">
          <div className="servers-grid">
            {serverList.length > 0 ? (
              serverList.map((server) => (
                <ServerCard
                  key={server.server_id}
                  server={server}
                  newPlayers={newPlayers}
                  isSelected={selectedServerId === server.server_id}
                  onSelect={
                    auth.isAuthenticated
                      ? () => handleServerSelect(server.server_id)
                      : undefined
                  }
                  onPlayerClick={handlePlayerClick}
                />
              ))
            ) : (
              <div className="loading">No servers available</div>
            )}
          </div>

          <RecentMatches onPlayerClick={handlePlayerClick} />
        </div>
      </div>

      {auth.isAuthenticated && showRcon && (
        <RconSidebar
          server={selectedServer}
          token={auth.token!}
          onClose={() => setShowRcon(false)}
        />
      )}

      {selectedPlayer && (
        <PlayerStatsModal
          playerName={selectedPlayer.name}
          playerId={selectedPlayer.playerId}
          onClose={() => setSelectedPlayer(null)}
        />
      )}

      {showPasswordChange && (
        <PasswordChangeModal
          required={auth.passwordChangeRequired}
          onPasswordChange={changePassword}
          onClose={() => setShowPasswordChange(false)}
        />
      )}

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

export default App;
