import { useState, useMemo } from "react";
import { useAuth } from "./hooks/useAuth";
import { useLiveData } from "./contexts/LiveDataContext";
import {
  ServerCard,
  RecentMatches,
  RconSidebar,
  ServerFilters,
} from "./components";
import {
  applyServerFilters,
  loadServerFilters,
  type ServerFilterState,
} from "./components/ServerFilters";
import { Header } from "./components/Header";

function App() {
  const { auth } = useAuth();
  const {
    servers, liveness, manageable, newPlayers, loading,
    showPlayer,
  } = useLiveData();

  // Home-route-specific state: server selection drives the RCON sidebar,
  // and the on-page server filter strip is local to the home grid.
  const [selectedServerId, setSelectedServerId] = useState<number | null>(null);
  const [showRcon, setShowRcon] = useState(false);
  const [serverFilters, setServerFilters] = useState<ServerFilterState>(() => loadServerFilters());

  const handleServerSelect = (serverId: number) => {
    setSelectedServerId(serverId);
    if (auth.isAuthenticated) {
      setShowRcon(true);
    }
  };

  const selectedServer = useMemo(
    () => (selectedServerId !== null ? servers.get(selectedServerId) || null : null),
    [selectedServerId, servers],
  );

  if (loading) {
    return (
      <div className="app">
        <Header title="Trinity" className="app-header" wordmark="tracker" />
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
      <Header title="Trinity" className="app-header" wordmark="tracker" />

      <div className="app-layout">
        <div className="main-content">
          <ServerFilters
            servers={fullServerList}
            filters={serverFilters}
            onChange={setServerFilters}
          />
          <div className="servers-grid">
            {serverList.length > 0 ? (
              serverList.map((server) => (
                <ServerCard
                  key={server.server_id}
                  server={server}
                  newPlayers={newPlayers}
                  isSelected={selectedServerId === server.server_id}
                  onSelect={
                    manageable.get(server.server_id)
                      ? () => handleServerSelect(server.server_id)
                      : undefined
                  }
                  onPlayerClick={showPlayer}
                  liveness={liveness.get(server.server_id)}
                />
              ))
            ) : fullServerList.length > 0 ? (
              <div className="loading">No servers match the current filters</div>
            ) : (
              <div className="loading">No servers available</div>
            )}
          </div>

          <RecentMatches onPlayerClick={showPlayer} />
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
