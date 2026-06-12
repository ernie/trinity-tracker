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
  // Stream is /stream-suffixed; the bare path is the SPA player (full-doc load).
  const liveUrl = `/tv/${source}/${key}/stream`;

  // This server's already-polled /api/servers status: the map drives the loading
  // levelshot backdrop (instant, no waiting on the delayed stream header); the
  // viewer delay is surfaced on LIVE-badge hover so the feed's lag is honest.
  const { servers } = useLiveData();
  const liveServer = useMemo(() => {
    for (const s of servers.values()) {
      if (s.source === source && s.key === key) return s;
    }
    return undefined;
  }, [servers, source, key]);
  const liveMap = liveServer?.map;
  const liveDelay = liveServer?.live_delay_seconds;
  // Viewer count includes this viewer; heartbeat-fresh (~30s), like is_live.
  const liveViewers = liveServer?.live_viewers ?? 0;
  const liveBadgeDetails = [
    ...(liveViewers > 0 ? [`${liveViewers} watching`] : []),
    ...(liveDelay ? [`delayed ~${liveDelay}s`] : []),
  ].join(", ");

  // ?f=<clientnum>: open already following the leader the server card picked at
  // click time. Captured once — a client slot is only meaningful "now", so a
  // reconnect (which fully reloads the page) must not re-apply a now-stale slot.
  // We scrub ?f from the URL after reading it, leaving a clean URL for the
  // reconnect reload to land on. (Match cards never emit ?f for this reason.)
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

  // useReducer for a stable dispatch; each transition's action runs in the effect below.
  const [sm, dispatchEvent] = useReducer(reduce, undefined, () =>
    initialLive(Date.now()),
  );
  const [playerListOpen, setPlayerListOpen] = useState(false);

  // Connection-health verdict for THIS page life. Each abrupt reconnect reloads
  // the page (see the reboot action below), so the drop history lives in
  // sessionStorage and we read it once at mount — a fresh mount per drop keeps it
  // current without polling. True ⇒ enough recent drops to warn the user.
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

  // Perform the reducer's one action per transition; each branch's follow-up
  // dispatchEvent advances the machine, re-running this.
  useEffect(() => {
    const a = sm.action;
    if (!a) return;
    if (a.kind === "probe") {
      // The relay answers 200 (TVL1) when live, 503 in the gap. We only need
      // the status, so cancel before draining the (endless) stream body.
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
          // The success path and effect cleanup both abort, rejecting with
          // AbortError — intentional cancellation, not a failed probe, so don't
          // feed it back as status:0.
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
      // An abrupt reconnect (stream was still live — our connection dropped, not a
      // confirmed inter-match gap) is a connection-health signal; stamp it before
      // the reload so a pattern of drops survives into the next page life and can
      // escalate the warning. A clean next-match advance (gapConfirmed) is not.
      if (!sm.gapConfirmed) recordDrop(Date.now());
      // Full reload onto the next match, not an in-document re-boot — each
      // engine needs a fresh document (iOS can't hold two WASM heaps).
      window.location.reload();
      return;
    }
    // "showVod" needs no side effect — the render branch handles it.
  }, [sm, liveUrl]);

  // A live-mode boot failure (loader throws on a non-200 open — an inter-match
  // gap, or a transient nginx 502 at the close→reconnect transition) must poll
  // via the SM, not dead-end: feed it in as streamEnded. Latch per error episode
  // because the hook clears `error` only after an awaited re-import, so on a
  // 200→reboot recovery sm.phase flips to "live" while `error` is still
  // stale-truthy — without the latch that fires a spurious second streamEnded.
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

  // Two terminal states after we give up reconnecting (see reconnect.ts): "ended"
  // means the relay confirmed the match is over; "lost" means we never reached the
  // relay (the viewer's connection dropped) — don't claim the match finished, and
  // offer a manual retry since we've stopped auto-probing.
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
  // Engine-reported map wins (crisp, in-sync with an in-place change); the
  // /api/servers map seeds the backdrop before the engine has reported one.
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
      {/* transitioning = an in-place map change (engine cl_tvMapName) — genuinely
          loading the next map. reconnecting = the stream drained and we're re-
          probing the relay: a non-200 answer (gapConfirmed) means the match ended
          and we're between matches; otherwise the stream is still live and it was
          OUR connection that dropped — so don't claim a map change. */}
      {transitioning ? (
        <div className="demo-status">Loading next map…</div>
      ) : reconnecting ? (
        <div className="demo-status">
          {sm.gapConfirmed ? "Waiting for the next match…" : "Reconnecting…"}
        </div>
      ) : null}
      {/* Repeated drop→reload cycles this session ⇒ name the likely cause. Shown
          only while a status overlay is up (booting or reconnecting) — exactly when
          the user is waiting and wondering why; hidden once playback resumes. */}
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
      {/* Map levelshot backdrop while the engine boots, between maps (VOD-fallback
          reconnect), and across an in-place map change — covering the blank/frozen
          canvas and the engine's "Connection Interrupted" during a pk3 download. The
          engine-reported map (liveMapName) is authoritative and in-sync with the
          actual change; fall back to the /api/servers map before the engine reports. */}
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
        {/* View: POV + free-cam only (no transport/scrub/share in live) */}
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
