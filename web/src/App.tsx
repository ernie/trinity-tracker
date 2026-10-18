import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useMemo,
} from "react";
import { useAuth } from "./hooks/useAuth";
import { useLiveData } from "./contexts/LiveDataContext";
import { ServerCard, RconSidebar, ServerFilters } from "./components";
import {
  applyServerFilters,
  loadServerFilters,
  type ServerFilterState,
} from "./components/ServerFilters";
import { HelpModeRoot } from "./components/HelpModeRoot";

// Clears the fixed status pill (top 22px, about 30px tall) with a small gap.
const HELP_TOGGLE_STICK_TOP_PX = 58;

function App() {
  const { auth } = useAuth();
  const { servers, liveness, manageable, loading, showPlayer } = useLiveData();

  const [selectedServerId, setSelectedServerId] = useState<number | null>(null);
  const [showRcon, setShowRcon] = useState(false);
  const [serverFilters, setServerFilters] = useState<ServerFilterState>(() =>
    loadServerFilters(),
  );
  const [helpMode, setHelpMode] = useState<boolean>(false);

  // Set only from the scroll rAF callback (react-hooks/set-state-in-effect).
  // The value left over while help mode is off is masked by effectiveFloatPos.
  const toolbarRef = useRef<HTMLDivElement>(null);
  const [floatPos, setFloatPos] = useState<{
    top: number;
    right: number;
  } | null>(null);

  useEffect(() => {
    if (!helpMode) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setHelpMode(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [helpMode]);

  // Measured from the toolbar: when help mode turns back on, the button is
  // still at its old fixed position, and the toolbar rect never moves with it.
  useLayoutEffect(() => {
    if (!helpMode || !toolbarRef.current) return;
    const toolbarRect = toolbarRef.current.getBoundingClientRect();
    const naturalDocY = toolbarRect.top + window.scrollY;
    // The toolbar rect and a fixed `right` both exclude the scrollbar, as
    // clientWidth does; innerWidth can include it.
    const naturalRight =
      document.documentElement.clientWidth - toolbarRect.right;

    let raf = 0;
    const update = () => {
      raf = 0;
      setFloatPos({
        top: Math.max(HELP_TOGGLE_STICK_TOP_PX, naturalDocY - window.scrollY),
        right: naturalRight,
      });
    };
    raf = requestAnimationFrame(update);
    const onScroll = () => {
      if (raf) return;
      raf = requestAnimationFrame(update);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.removeEventListener("scroll", onScroll);
      if (raf) cancelAnimationFrame(raf);
    };
  }, [helpMode]);

  const effectiveFloatPos = helpMode ? floatPos : null;

  const handleServerSelect = useCallback(
    (serverId: number) => {
      setSelectedServerId(serverId);
      if (auth.isAuthenticated) {
        setShowRcon(true);
      }
    },
    [auth.isAuthenticated],
  );

  const selectedServer = useMemo(
    () =>
      selectedServerId !== null ? servers.get(selectedServerId) || null : null,
    [selectedServerId, servers],
  );

  if (loading) {
    return (
      <div className="app">
        <div className="loading">Loading servers…</div>
      </div>
    );
  }

  const fullServerList = Array.from(servers.values()).sort(
    (a, b) => a.server_id - b.server_id,
  );
  const serverList = applyServerFilters(fullServerList, serverFilters);

  return (
    <div
      className={`app ${showRcon && auth.isAuthenticated ? "with-right-sidebar" : ""}`}
    >
      <div className="app-layout">
        <div className="main-content">
          <div className="servers-toolbar" ref={toolbarRef}>
            <ServerFilters
              servers={fullServerList}
              filters={serverFilters}
              onChange={setServerFilters}
            />
            <button
              type="button"
              className={`servers-help-toggle${helpMode ? " active" : ""}${effectiveFloatPos ? " floating" : ""}`}
              style={
                effectiveFloatPos
                  ? {
                      top: `${effectiveFloatPos.top}px`,
                      right: `${effectiveFloatPos.right}px`,
                    }
                  : undefined
              }
              onClick={() => setHelpMode((h) => !h)}
              aria-pressed={helpMode}
              title={
                helpMode
                  ? "Turn off help and hide the card tooltips"
                  : "Turn on help, then hover any part of a card to see what it means"
              }
            >
              <span aria-hidden="true" className="servers-help-toggle__glyph">
                ?
              </span>
              <span className="servers-help-toggle__label">
                {helpMode ? "Exit help" : "What's this?"}
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
                      onSelect={
                        !helpMode && manageable.get(server.server_id)
                          ? handleServerSelect
                          : undefined
                      }
                      onPlayerClick={helpMode ? undefined : showPlayer}
                      liveness={liveness.get(server.server_id)}
                    />
                  ))
                ) : fullServerList.length > 0 ? (
                  <div className="loading">No servers match these filters</div>
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
          onClose={() => setShowRcon(false)}
        />
      )}
    </div>
  );
}

export default App;
