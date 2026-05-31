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

// Sticky-stop offset for the floating help toggle. The chrome status
// pill is the only thing in this corner that remains fixed during
// scroll — sits at top:22 (16 on mobile), ~30px tall, so bottom is
// ~52 / ~46. The header search box scrolls away with the page, so
// don't bother clearing it. ~6px gap below the pill on desktop.
const HELP_TOGGLE_STICK_TOP_PX = 58;

function App() {
  const { auth } = useAuth();
  const { servers, liveness, manageable, loading, showPlayer } = useLiveData();

  // Home-route-specific state: server selection drives the RCON sidebar,
  // and the on-page server filter strip is local to the home grid.
  const [selectedServerId, setSelectedServerId] = useState<number | null>(null);
  const [showRcon, setShowRcon] = useState(false);
  const [serverFilters, setServerFilters] = useState<ServerFilterState>(() =>
    loadServerFilters(),
  );
  // Help mode wraps the grid in <HelpModeRoot> so the data-help
  // attributes on ServerCard internals (state badge, flag indicators,
  // obelisk HP, skull counts, etc.) light up as hover/focus/tap
  // popovers. Ephemeral: starts off on every page load.
  const [helpMode, setHelpMode] = useState<boolean>(false);

  // Floating position for the help toggle. Owned by a scroll-driven
  // rAF callback; never set synchronously inside an effect (rule:
  // react-hooks/set-state-in-effect). Old values persist when help
  // mode flips off and are masked by `effectiveFloatPos` below.
  const toolbarRef = useRef<HTMLDivElement>(null);
  const [floatPos, setFloatPos] = useState<{
    top: number;
    right: number;
  } | null>(null);

  // Escape exits help mode — the toggle is floated when active, but a
  // keyboard escape is faster than a mouse hop and a more familiar
  // idiom for dismissing a transient mode.
  useEffect(() => {
    if (!helpMode) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setHelpMode(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [helpMode]);

  // Anchor the floating toggle to the TOOLBAR, not the button itself.
  // Measuring the button would drift across toggles: on re-activation
  // the button is still in its stale fixed-position from the previous
  // cycle (effectiveFloatPos echoes the stale floatPos the moment
  // helpMode flips back on), so getBoundingClientRect() on the button
  // would read the floating box, not the natural toolbar slot. The
  // toolbar rect is invariant of the button's flow/fixed state and of
  // its label width (margin-left:auto pushes the button's right edge
  // to the toolbar's right edge in any flex flow), so it's the
  // stable anchor for both the rise-with-scroll math and the
  // right-edge offset.
  useLayoutEffect(() => {
    if (!helpMode || !toolbarRef.current) return;
    const toolbarRect = toolbarRef.current.getBoundingClientRect();
    const naturalDocY = toolbarRect.top + window.scrollY;
    // Use documentElement.clientWidth, not window.innerWidth — both
    // toolbarRect and position:fixed `right` are resolved against the
    // layout viewport (scrollbar excluded), and innerWidth can
    // include the scrollbar gutter on some browsers/configs. The
    // mismatch would land the floating button a scrollbar-width to
    // the left of the toolbar's right edge.
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

  // Mask stale floatPos left over from a previous activation when
  // help mode is currently off.
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
        <div className="loading">Loading servers...</div>
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
                  ? "Turn off help — hide tooltips on cards"
                  : "Turn on help — hover any card piece to learn what it means"
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
                  <div className="loading">
                    No servers match the current filters
                  </div>
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
