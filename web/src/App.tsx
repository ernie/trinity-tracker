import { useCallback, useState, useMemo } from "react";
import { useAuth } from "./hooks/useAuth";
import { useLiveData } from "./contexts/LiveDataContext";
import {
  ServerCard,
  RconSidebar,
  ServerFilters,
} from "./components";
import {
  applyServerFilters,
  loadServerFilters,
  type ServerFilterState,
} from "./components/ServerFilters";
import { HelpModeRoot } from "./components/HelpModeRoot";

function App() {
  const { auth } = useAuth();
  const {
    servers, liveness, manageable, loading,
    showPlayer,
  } = useLiveData();

  // Home-route-specific state: server selection drives the RCON sidebar,
  // and the on-page server filter strip is local to the home grid.
  const [selectedServerId, setSelectedServerId] = useState<number | null>(null);
  const [showRcon, setShowRcon] = useState(false);
  const [serverFilters, setServerFilters] = useState<ServerFilterState>(() => loadServerFilters());
  // Help mode wraps the grid in <HelpModeRoot> so the data-help
  // attributes on ServerCard internals (state badge, flag indicators,
  // obelisk HP, skull counts, etc.) light up as hover/focus/tap
  // popovers. Ephemeral: starts off on every page load.
  const [helpMode, setHelpMode] = useState<boolean>(false);

  const handleServerSelect = useCallback((serverId: number) => {
    setSelectedServerId(serverId);
    if (auth.isAuthenticated) {
      setShowRcon(true);
    }
  }, [auth.isAuthenticated]);

  const selectedServer = useMemo(
    () => (selectedServerId !== null ? servers.get(selectedServerId) || null : null),
    [selectedServerId, servers],
  );

  if (loading) {
    return (
      <div className="app">
        <div className="loading">Loading servers...</div>
      </div>
    );
  }

  const fullServerList = Array.from(servers.values()).sort(
    (a, b) => a.server_id - b.server_id,
  );
  const serverList = applyServerFilters(fullServerList, serverFilters);

  return (
    <div className={`app ${showRcon && auth.isAuthenticated ? "with-right-sidebar" : ""}`}>
      <div className="app-layout">
        <div className="main-content">
          <div className="servers-toolbar">
            <ServerFilters
              servers={fullServerList}
              filters={serverFilters}
              onChange={setServerFilters}
            />
            <button
              type="button"
              className={`servers-help-toggle ${helpMode ? "active" : ""}`}
              onClick={() => setHelpMode((h) => !h)}
              aria-pressed={helpMode}
              title={helpMode
                ? "Turn off help — hide tooltips on cards"
                : "Turn on help — hover any card piece to learn what it means"}
            >
              <span aria-hidden="true" className="servers-help-toggle__glyph">?</span>
              <span className="servers-help-toggle__label">
                {helpMode ? "Hide help" : "What's this?"}
              </span>
            </button>
          </div>
          {(() => {
            const grid = (
              <div className="servers-grid">
                {serverList.length > 0 ? (
                  serverList.map((server) => (
                    <ServerCard
                      key={server.server_id}
                      server={server}
                      isSelected={selectedServerId === server.server_id}
                      onSelect={!helpMode && manageable.get(server.server_id) ? handleServerSelect : undefined}
                      onPlayerClick={helpMode ? undefined : showPlayer}
                      liveness={liveness.get(server.server_id)}
                    />
                  ))
                ) : fullServerList.length > 0 ? (
                  <div className="loading">No servers match the current filters</div>
                ) : (
                  <div className="loading">No servers available</div>
                )}
              </div>
            );
            return helpMode ? <HelpModeRoot>{grid}</HelpModeRoot> : grid;
          })()}
        </div>
      </div>

      {auth.isAuthenticated && showRcon && (
        <RconSidebar
          server={selectedServer}
          token={auth.token!}
          onClose={() => setShowRcon(false)}
        />
      )}
    </div>
  );
}

export default App;
