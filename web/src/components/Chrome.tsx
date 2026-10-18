import { useCallback } from "react";
import { useLocation } from "react-router-dom";
import { StatusPill } from "./StatusPill";
import { ActivityDrawer } from "./ActivityDrawer";
import { PlayerStatsModal } from "./PlayerStatsModal";
import { PasswordChangeModal } from "./PasswordChangeModal";
import { CommandPalette } from "./CommandPalette";
import { useLiveData } from "../contexts/LiveDataContext";
import { useAuth } from "../hooks/useAuth";
import { useHotkey } from "../hooks/useHotkey";

export function Chrome() {
  const live = useLiveData();
  const { auth, changePassword } = useAuth();
  const { pathname } = useLocation();

  // The landing hero draws its own pill, and the game canvas pages own
  // the whole viewport.
  const hidePill =
    pathname === "/" ||
    pathname === "/play" ||
    /^\/matches\/\d+\/demo$/.test(pathname) ||
    pathname.startsWith("/tv/");

  useHotkey(
    { key: "k", meta: true },
    useCallback(() => {
      live.setCommandPaletteOpen(true);
    }, [live]),
  );

  return (
    <>
      {!hidePill && (
        <div className="chrome-top-right" aria-hidden={false}>
          <StatusPill
            humansOnline={live.activeHumanPlayersCount}
            activeServers={live.activeServersCount}
            status={live.connectionStatus}
            open={live.activityDrawerOpen}
            onToggle={live.toggleActivityDrawer}
          />
        </div>
      )}

      {live.activityDrawerOpen && (
        <ActivityDrawer
          activities={live.activities}
          servers={live.servers}
          onClose={() => live.setActivityDrawerOpen(false)}
          onPlayerClick={live.showPlayer}
        />
      )}

      {live.selectedPlayer && (
        <PlayerStatsModal
          playerName={live.selectedPlayer.name}
          playerId={live.selectedPlayer.playerId}
          onClose={live.closePlayer}
        />
      )}

      {live.showPasswordChange && (
        <PasswordChangeModal
          required={auth.passwordChangeRequired}
          onPasswordChange={changePassword}
          onClose={() => live.setShowPasswordChange(false)}
        />
      )}

      <CommandPalette
        open={live.commandPaletteOpen}
        onClose={() => live.setCommandPaletteOpen(false)}
      />
    </>
  );
}
