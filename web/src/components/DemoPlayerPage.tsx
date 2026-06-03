import { useEffect, useRef, useState, useCallback } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { ColoredText } from "./ColoredText";
import { EngineVolume } from "./EngineVolume";
import { PlayerPortrait } from "./PlayerPortrait";
import { TvMovePad } from "./TvMovePad";
import { useTvEngine } from "../hooks/useTvEngine";
import { parseTimeParam, formatTimeParam, stripVRPrefix } from "../utils";

// Parse a follow-target client number from the URL. Out-of-range or
// non-numeric returns null so the receiver silently ignores garbage links.
function parseClientNum(raw: string): number | null {
  if (!/^\d+$/.test(raw)) return null;
  const n = Number(raw);
  return n >= 0 && n < 64 ? n : null;
}

interface MatchData {
  id: number;
  map_name: string;
  demo_url?: string;
}

export function DemoPlayerPage() {
  const { id } = useParams<{ id: string }>();

  const [demoUrl, setDemoUrl] = useState<string | undefined>(undefined);
  const [mapName, setMapName] = useState<string | null>(null);
  const [fetchError, setFetchError] = useState<string | null>(null);
  // Declared before the hook so it can wire the canvas-touch overlay dismiss.
  const [playerListOpen, setPlayerListOpen] = useState(false);

  const {
    canvasRef,
    statusRef,
    moduleRef,
    engineReady,
    loading,
    progress,
    error: engineError,
    playerList,
    viewpoint,
    refreshPlayerList,
    follow,
    sendKey,
    sendMouse,
    preventFocus,
    holdHandlers,
    scrubActive,
    setScrub,
  } = useTvEngine({
    demoUrl,
    extraArgs: "+set cl_demoPlayer 1",
    onCanvasTouchStart: () => setPlayerListOpen(false),
  });
  const error = fetchError ?? engineError;

  const playerWrapRef = useRef<HTMLDivElement>(null);
  const [shareCopied, setShareCopied] = useState(false);

  // Deep-link URL params: ?t=<time>&f=<clientnum>. Applied once after the
  // engine is live and the first snapshot has arrived (tv_view needs the
  // player table populated to validate the target).
  const [search] = useSearchParams();
  const initialSeek = parseTimeParam(search.get("t") ?? "");
  const initialFollow = parseClientNum(search.get("f") ?? "");
  const hasDeepLink = initialSeek !== null || initialFollow !== null;
  const paramsAppliedRef = useRef(false);
  // Keep the levelshot covering the canvas while we wait to seek/follow,
  // so the viewer never sees the demo's default viewpoint flash.
  const [paramsApplied, setParamsApplied] = useState(!hasDeepLink);

  // Fetch the match to resolve the demo URL + map name; setting demoUrl boots
  // the engine via useTvEngine (its boot effect no-ops until demoUrl is defined).
  useEffect(() => {
    let aborted = false;
    (async () => {
      try {
        const resp = await fetch(`/api/matches/${id}`);
        if (aborted) return;
        if (!resp.ok) {
          setFetchError(`Match not found (${resp.status})`);
          return;
        }
        const match: MatchData = await resp.json();
        if (aborted) return;
        setMapName(match.map_name);
        if (!match.demo_url) {
          setFetchError("No demo available for this match");
          return;
        }
        setDemoUrl(match.demo_url);
      } catch (e) {
        if (!aborted)
          setFetchError(e instanceof Error ? e.message : "Failed to load demo");
      }
    })();
    return () => {
      aborted = true;
    };
  }, [id]);

  // Safari doesn't deliver arrow key events through Emscripten's SDL2 keyboard path
  // when pointer lock is active.  Intercept them in JS and inject via Cbuf_AddText.
  useEffect(() => {
    const arrowCmds: Record<string, string> = {
      ArrowUp: "followrecenter",
      ArrowDown: "demopause",
      ArrowLeft: "tv_backward",
      ArrowRight: "tv_forward",
    };
    const onDown = (e: KeyboardEvent) => {
      const cmd = arrowCmds[e.code];
      if (!cmd || !document.pointerLockElement) return;
      const mod = moduleRef.current;
      if (!mod?.ccall) return;
      e.preventDefault();
      e.stopImmediatePropagation();
      mod.ccall("Cbuf_AddText", null, ["string"], [cmd + "\n"]);
    };
    window.addEventListener("keydown", onDown, true);
    return () => window.removeEventListener("keydown", onDown, true);
  }, [moduleRef]);

  // Apply ?t=&f= deep-link params once the engine is live and a snapshot
  // has arrived. We poll CL_TV_GetPlayerList because there's no JS-side
  // "first snap" hook — the player list is empty until the engine has
  // processed at least one snapshot, which is when tv_view's target table
  // is valid.
  useEffect(() => {
    if (!engineReady || paramsAppliedRef.current) return;
    if (initialSeek === null && initialFollow === null) {
      paramsAppliedRef.current = true;
      return;
    }
    const mod = moduleRef.current;
    if (!mod?.ccall) return;
    const interval = setInterval(() => {
      let raw: string | null = null;
      try {
        raw = mod.ccall("CL_TV_GetPlayerList", "string", [], []);
      } catch (e) {
        console.debug("CL_TV_GetPlayerList failed", e);
        return;
      }
      if (!raw) return;
      const lines = raw.split("\n").filter(Boolean);
      if (lines.length < 2) return;
      paramsAppliedRef.current = true;
      clearInterval(interval);
      // follow() issues tv_view + tracks viewpoint; tv_seek is VOD-only.
      if (initialFollow !== null) follow(initialFollow);
      if (initialSeek !== null) {
        try {
          mod.ccall(
            "Cbuf_AddText",
            null,
            ["string"],
            [`tv_seek ${initialSeek}\n`],
          );
        } catch (e) {
          console.debug("apply URL params failed", e);
        }
      }
      // Wait one engine frame so the cbuf processes tv_view + tv_seek before
      // we drop the levelshot and reveal the canvas.
      mod.onNextFrame?.(() => setParamsApplied(true));
    }, 100);
    return () => clearInterval(interval);
  }, [engineReady, initialSeek, initialFollow, follow, moduleRef]);

  // Close player list when tapping outside on mobile
  useEffect(() => {
    if (!playerListOpen) return;
    const handler = (e: TouchEvent) => {
      if (
        playerWrapRef.current &&
        !playerWrapRef.current.contains(e.target as Node)
      ) {
        setPlayerListOpen(false);
      }
    };
    document.addEventListener("touchstart", handler);
    return () => document.removeEventListener("touchstart", handler);
  }, [playerListOpen]);

  const onShare = useCallback(() => {
    const mod = moduleRef.current;
    if (!mod?.ccall) return;
    let tMs = 0,
      fNum = -1;
    try {
      tMs = mod.ccall(
        "Cvar_VariableValue",
        "number",
        ["string"],
        ["cl_tvTime"],
      );
    } catch (e) {
      console.debug("cl_tvTime read failed", e);
    }
    try {
      fNum = mod.ccall(
        "Cvar_VariableValue",
        "number",
        ["string"],
        ["cl_tvViewpoint"],
      );
    } catch (e) {
      console.debug("cl_tvViewpoint read failed", e);
    }
    const params = new URLSearchParams();
    // Drop near-start times so the common "from the beginning" share stays clean.
    if (tMs >= 1000) params.set("t", formatTimeParam(tMs / 1000));
    if (fNum >= 0 && fNum < 64) params.set("f", String(fNum | 0));
    const qs = params.toString();
    const url = `${window.location.origin}${window.location.pathname}${qs ? "?" + qs : ""}`;
    navigator.clipboard
      .writeText(url)
      .then(() => {
        setShareCopied(true);
        setTimeout(() => setShareCopied(false), 1500);
      })
      .catch((e) => console.debug("clipboard write failed", e));
  }, [moduleRef]);

  const handleScrubToggle = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setScrub(!scrubActive);
    },
    [setScrub, scrubActive],
  );

  if (error) {
    return (
      <div className="demo-player-page">
        <div className="demo-player-error">
          <p>{error}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="demo-player-page">
      <canvas
        ref={canvasRef}
        id="canvas"
        tabIndex={0}
        className="demo-canvas"
      />
      <div ref={statusRef} className="demo-status">
        {loading ? "Loading..." : ""}
      </div>
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
      {mapName && (!engineReady || !paramsApplied) && (
        <div
          className="demo-levelshot"
          style={{
            backgroundImage: `url(/assets/levelshots/${mapName.toLowerCase()}.jpg)`,
          }}
        />
      )}

      <div
        className="demo-controls-bar"
        onContextMenu={(e) => e.preventDefault()}
      >
        {/* Transport */}
        <div className="ctrl-group">
          <button
            className="ctrl-btn"
            title="Rewind"
            {...holdHandlers(
              () => sendKey("ArrowLeft", "keydown"),
              () => sendKey("ArrowLeft", "keyup"),
            )}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
              <path d="M11 18V6l-7 6 7 6zm7 0V6l-7 6 7 6z" />
            </svg>
          </button>
          <button
            className="ctrl-btn"
            title="Pause"
            onMouseDown={preventFocus}
            onClick={() => {
              sendKey("ArrowDown", "keydown");
              setTimeout(() => sendKey("ArrowDown", "keyup"), 50);
            }}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
              <path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z" />
            </svg>
          </button>
          <button
            className="ctrl-btn"
            title="Forward"
            {...holdHandlers(
              () => sendKey("ArrowRight", "keydown"),
              () => sendKey("ArrowRight", "keyup"),
            )}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
              <path d="M6 18V6l7 6-7 6zm7 0V6l7 6-7 6z" />
            </svg>
          </button>
        </div>

        {/* View */}
        <div className="ctrl-group">
          <div
            ref={playerWrapRef}
            className={`ctrl-player-wrap${playerListOpen ? " open" : ""}`}
            onMouseEnter={refreshPlayerList}
          >
            <button
              className="ctrl-btn"
              title="Follow player"
              onMouseDown={preventFocus}
              onTouchStart={(e) => {
                e.preventDefault();
                refreshPlayerList();
                setPlayerListOpen((prev) => !prev);
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
                    if (p.team === 3) return;
                    follow(p.clientNum);
                  }}
                  onMouseUp={(e) => e.stopPropagation()}
                  onClick={(e) => e.stopPropagation()}
                  onTouchStart={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    if (p.team === 3) return;
                    follow(p.clientNum);
                  }}
                >
                  <span className="ctrl-player-vr-slot">
                    {p.isVR && <img src="/assets/vr/vr.png" alt="VR" />}
                  </span>
                  <PlayerPortrait model={p.model} size="sm" />
                  <ColoredText text={p.isVR ? stripVRPrefix(p.name) : p.name} />
                </button>
              ))}
            </div>
          </div>
          <button
            className="ctrl-btn"
            title="Toggle camera"
            onMouseDown={preventFocus}
            onClick={() => {
              sendMouse(2, "mousedown");
              setTimeout(() => sendMouse(2, "mouseup"), 50);
            }}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
              <path d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z" />
            </svg>
          </button>
          <button
            className="ctrl-btn"
            title="Recenter view"
            onMouseDown={preventFocus}
            onClick={() => {
              sendKey("ArrowUp", "keydown");
              setTimeout(() => sendKey("ArrowUp", "keyup"), 50);
            }}
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
        </div>

        {/* Toggle / Hold */}
        <div className="ctrl-group">
          <button
            className={`ctrl-btn${scrubActive ? " active" : ""}`}
            title="Scrub mode"
            onMouseDown={handleScrubToggle}
          >
            <svg
              viewBox="0 0 24 24"
              width="18"
              height="18"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
              <line x1="3" y1="12" x2="21" y2="12" />
              <circle cx="12" cy="12" r="3" fill="currentColor" />
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

        <div className="ctrl-group ctrl-share-wrap">
          <button
            className={`ctrl-btn${shareCopied ? " active" : ""}`}
            title={
              shareCopied ? "Link copied!" : "Copy share link to this moment"
            }
            onMouseDown={preventFocus}
            onClick={onShare}
          >
            {shareCopied ? (
              <svg
                viewBox="0 0 24 24"
                width="18"
                height="18"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M5 12l5 5L20 7" />
              </svg>
            ) : (
              <svg
                viewBox="0 0 24 24"
                width="18"
                height="18"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M16 6l-4-4-4 4" />
                <line x1="12" y1="2" x2="12" y2="15" />
                <path d="M6 10H5a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-8a2 2 0 0 0-2-2h-1" />
              </svg>
            )}
          </button>
        </div>

        <div className="ctrl-group ctrl-help-wrap">
          <button className="ctrl-btn ctrl-help-btn" onMouseDown={preventFocus}>
            ?
          </button>
          <div className="ctrl-help-tooltip">
            <div>
              <kbd>←</kbd> / <kbd>→</kbd> Rewind / Forward
            </div>
            <div>
              <kbd>↓</kbd> Pause
            </div>
            <div>
              <kbd>↑</kbd> Recenter view
            </div>
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
              <kbd>Shift + Click</kbd> Scrub timeline
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
      </div>

      <TvMovePad sendKey={sendKey} holdHandlers={holdHandlers} />
    </div>
  );
}
