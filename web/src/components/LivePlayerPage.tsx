import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { ColoredText } from "./ColoredText";
import { PlayerPortrait } from "./PlayerPortrait";
import { EngineVolume } from "./EngineVolume";
import { TvMovePad } from "./TvMovePad";
import { useTvEngine } from "../hooks/useTvEngine";
import { useLiveData } from "../contexts/LiveDataContext";
import { parseClientNum, stripVRPrefix } from "../utils";
import { initialLive, reduce } from "./live/reconnect";
import { connectionUnstable, recordDrop } from "./live/connectionHealth";

export function LivePlayerPage() {
  const { source, key } = useParams<{ source: string; key: string }>();
  // The bare /tv path is this page; the relay serves the stream under /stream.
  const liveUrl = `/tv/${source}/${key}/stream`;

  // /api/servers has the map before the delayed stream header arrives.
  const { servers } = useLiveData();
  const liveServer = useMemo(() => {
    for (const s of servers.values()) {
      if (s.source === source && s.key === key) return s;
    }
    return undefined;
  }, [servers, source, key]);
  const liveMap = liveServer?.map;
  const liveDelay = liveServer?.live_delay_seconds;
  // Includes this viewer.
  const liveViewers = liveServer?.live_viewers ?? 0;
  const liveBadgeDetails = [
    ...(liveViewers > 0 ? [`${liveViewers} watching`] : []),
    ...(liveDelay ? [`delayed ~${liveDelay}s`] : []),
  ].join(", ");

  // ?f=<clientnum> opens following the player the server card picked. It's read
  // once and scrubbed, since a reconnect reloads the page and the slot goes stale.
  const [searchParams, setSearchParams] = useSearchParams();
  const [initialFollow] = useState(() =>
    parseClientNum(searchParams.get("f") ?? ""),
  );
  useEffect(() => {
    if (!searchParams.has("f")) return;
    const next = new URLSearchParams(searchParams);
    next.delete("f");
    setSearchParams(next, { replace: true });
  }, [searchParams, setSearchParams]);

  const [sm, dispatchEvent] = useReducer(reduce, undefined, () =>
    initialLive(Date.now()),
  );
  const [playerListOpen, setPlayerListOpen] = useState(false);

  // Each abrupt reconnect reloads the page, so drop history lives in
  // sessionStorage and is read once per mount.
  const [unstableConnection] = useState(() => connectionUnstable(Date.now()));

  const {
    canvasRef,
    statusRef,
    moduleRef,
    engineReady,
    loading,
    progress,
    error,
    playerList,
    viewpoint,
    liveMapName,
    transitioning,
    initialViewApplied,
    refreshPlayerList,
    follow,
    runCmd,
    sendKey,
    holdHandlers,
  } = useTvEngine({
    liveUrl,
    extraArgs: "+set cl_demoPlayer 1",
    initialFollow,
    onStreamClosed: () =>
      dispatchEvent({ type: "streamEnded", now: Date.now() }),
    onCanvasTouchStart: () => setPlayerListOpen(false),
  });

  // Runs the reducer's action for each transition; the follow-up dispatch
  // advances the machine.
  useEffect(() => {
    const a = sm.action;
    if (!a) return;
    if (a.kind === "probe") {
      // The relay answers 200 while live and 503 between matches. Only the
      // status matters, so abort before reading the endless body.
      const ctrl = new AbortController();
      fetch(liveUrl, { signal: ctrl.signal })
        .then((r) => {
          ctrl.abort();
          dispatchEvent({
            type: "probeResult",
            status: r.status,
            now: Date.now(),
          });
        })
        .catch((err) => {
          // The success path and the cleanup both abort on purpose.
          if (err?.name === "AbortError") return;
          dispatchEvent({ type: "probeResult", status: 0, now: Date.now() });
        });
      return () => ctrl.abort();
    }
    if (a.kind === "scheduleProbe") {
      const t = setTimeout(
        () => dispatchEvent({ type: "probeDue", now: Date.now() }),
        a.delayMs,
      );
      return () => clearTimeout(t);
    }
    if (a.kind === "reboot") {
      // A drop while the stream was still live is recorded before the reload,
      // so repeated drops can raise the unstable-connection warning.
      if (!sm.gapConfirmed) recordDrop(Date.now());
      // Each engine needs a fresh document; iOS can't hold two WASM heaps.
      window.location.reload();
      return;
    }
  }, [sm, liveUrl]);

  // A live boot failure (a non-200 open between matches, or a transient 502 at
  // reconnect) feeds the machine as streamEnded. The latch allows one per error:
  // the hook clears `error` only after an awaited re-import, so it can still be
  // set after the phase returns to live.
  const bootErrorHandledRef = useRef(false);
  useEffect(() => {
    if (!error) {
      bootErrorHandledRef.current = false;
      return;
    }
    if (sm.phase === "live" && !bootErrorHandledRef.current) {
      bootErrorHandledRef.current = true;
      dispatchEvent({ type: "streamEnded", now: Date.now() });
    }
  }, [error, sm.phase]);

  // "ended" means the relay confirmed the match is over; "lost" means the relay
  // was never reached, and probing has stopped, so the viewer gets a retry.
  if (sm.phase === "ended") {
    return (
      <div className="demo-player-page">
        <div className="demo-player-error">
          <p>This match has finished.</p>
        </div>
      </div>
    );
  }
  if (sm.phase === "lost") {
    return (
      <div className="demo-player-page">
        <div className="demo-player-error">
          <p>Connection lost.</p>
          <button
            className="demo-retry-btn"
            onClick={() => window.location.reload()}
          >
            Try again
          </button>
        </div>
      </div>
    );
  }

  const reconnecting = sm.phase === "probing" || sm.phase === "waiting";
  // The engine's map follows in-place changes; /api/servers covers the time
  // before the engine reports one.
  const backdropMap = liveMapName || liveMap;

  return (
    <div className="demo-player-page">
      <canvas
        ref={canvasRef}
        id="canvas"
        tabIndex={0}
        className="demo-canvas"
      />
      <div ref={statusRef} className="demo-status">
        {loading ? "Connecting…" : ""}
      </div>
      {/* gapConfirmed means the relay answered non-200, so the match ended;
          otherwise the viewer's own connection dropped. */}
      {transitioning ? (
        <div className="demo-status">Loading next map…</div>
      ) : reconnecting ? (
        <div className="demo-status">
          {sm.gapConfirmed ? "Waiting for the next match…" : "Reconnecting…"}
        </div>
      ) : null}
      {unstableConnection && (loading || reconnecting) && (
        <div className="demo-conn-warning">Your connection is unstable</div>
      )}
      {progress.total > 0 && (
        <div
          className="demo-progress"
          style={{ opacity: progress.loaded >= progress.total ? 0 : 1 }}
        >
          <div
            className="demo-progress-bar"
            style={{
              width: `${Math.min(100, (progress.loaded / progress.total) * 100)}%`,
            }}
          />
        </div>
      )}
      {/* The levelshot also covers the engine's "Connection Interrupted" during a
          pk3 download. */}
      {backdropMap &&
        (!engineReady ||
          reconnecting ||
          transitioning ||
          !initialViewApplied) && (
          <div
            className="demo-levelshot"
            style={{
              backgroundImage: `url(/assets/levelshots/${backdropMap.toLowerCase()}.jpg)`,
            }}
          />
        )}

      <div
        className="demo-controls-bar"
        onContextMenu={(e) => e.preventDefault()}
      >
        <div className="ctrl-group">
          <div
            className={`ctrl-player-wrap${playerListOpen ? " open" : ""}`}
            onMouseEnter={refreshPlayerList}
          >
            <button
              className="ctrl-btn"
              title="Follow player"
              onMouseDown={(e) => e.preventDefault()}
              onTouchStart={(e) => {
                e.preventDefault();
                refreshPlayerList();
                setPlayerListOpen((p) => !p);
              }}
            >
              <svg
                viewBox="0 0 24 24"
                width="18"
                height="18"
                fill="currentColor"
              >
                <path d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z" />
              </svg>
            </button>
            <div className="ctrl-player-list">
              {playerList.map((p) => (
                <button
                  key={p.clientNum}
                  className={`ctrl-player-item${p.clientNum === viewpoint ? " active" : ""}${p.team === 1 || p.team === 2 ? ` team-${p.team}` : ""}${p.team === 3 ? " spectator" : ""}`}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    if (p.team !== 3) follow(p.clientNum);
                  }}
                  onMouseUp={(e) => e.stopPropagation()}
                  onClick={(e) => e.stopPropagation()}
                  onTouchStart={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    if (p.team !== 3) follow(p.clientNum);
                  }}
                >
                  <span className="ctrl-player-vr-slot">
                    {p.isVR && <img src="/assets/vr/vr.png" alt="VR" />}
                  </span>
                  <PlayerPortrait model={p.model} team={p.team} size="sm" />
                  <ColoredText text={p.isVR ? stripVRPrefix(p.name) : p.name} />
                </button>
              ))}
            </div>
          </div>
          <button
            className="ctrl-btn"
            title="Toggle camera"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              canvasRef.current?.dispatchEvent(
                new MouseEvent("mousedown", { button: 2, bubbles: true }),
              );
              setTimeout(
                () =>
                  canvasRef.current?.dispatchEvent(
                    new MouseEvent("mouseup", { button: 2, bubbles: true }),
                  ),
                50,
              );
            }}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
              <path d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z" />
            </svg>
          </button>
          <button
            className="ctrl-btn"
            title="Recenter view"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => runCmd("followrecenter")}
          >
            <svg
              viewBox="0 0 24 24"
              width="18"
              height="18"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
              <circle cx="12" cy="12" r="3" />
              <line x1="12" y1="2" x2="12" y2="6" />
              <line x1="12" y1="18" x2="12" y2="22" />
              <line x1="2" y1="12" x2="6" y2="12" />
              <line x1="18" y1="12" x2="22" y2="12" />
            </svg>
          </button>
          <button
            className="ctrl-btn"
            title="Show scoreboard"
            {...holdHandlers(
              () => sendKey("Tab", "keydown"),
              () => sendKey("Tab", "keyup"),
            )}
          >
            <svg
              viewBox="0 0 24 24"
              width="18"
              height="18"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
              <rect x="3" y="3" width="18" height="18" rx="2" />
              <line x1="3" y1="9" x2="21" y2="9" />
              <line x1="3" y1="15" x2="21" y2="15" />
              <line x1="12" y1="3" x2="12" y2="21" />
            </svg>
          </button>
        </div>
        <EngineVolume moduleRef={moduleRef} engineReady={engineReady} />

        <div className="ctrl-group ctrl-help-wrap">
          <button
            className="ctrl-btn ctrl-help-btn"
            onMouseDown={(e) => e.preventDefault()}
          >
            ?
          </button>
          <div className="ctrl-help-tooltip">
            <div>
              <kbd>Click</kbd> Follow next player
            </div>
            <div>
              <kbd>Right-click</kbd> Toggle camera
            </div>
            <div>
              <kbd>Scroll</kbd> Zoom in/out
            </div>
            <div>
              <kbd>Tab</kbd> Scoreboard
            </div>
            <div>
              <kbd>W/A/S/D</kbd> Move camera
            </div>
            <div>
              <kbd>Space</kbd> / <kbd>C</kbd> Up / Down
            </div>
            <div>
              <kbd>Mouse</kbd> Look around
            </div>
          </div>
        </div>

        <span
          className="live-badge"
          aria-label={liveBadgeDetails ? `Live, ${liveBadgeDetails}` : "Live"}
          title={liveBadgeDetails || undefined}
        >
          <span className="live-badge__dot" aria-hidden="true" />
          LIVE
          {liveViewers > 0 && (
            <span className="live-badge__count" aria-hidden="true">
              {liveViewers}
            </span>
          )}
        </span>
      </div>

      <TvMovePad sendKey={sendKey} holdHandlers={holdHandlers} />
    </div>
  );
}
