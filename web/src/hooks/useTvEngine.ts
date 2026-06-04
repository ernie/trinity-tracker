import { useEffect, useRef, useState, useCallback } from "react";
import { flushSync } from "react-dom";
import type { EngineModule } from "../types";

export type SendKey = (code: string, type: "keydown" | "keyup") => void;
export type HoldHandlers = (
  downFn: () => void,
  upFn: () => void,
) => {
  onMouseDown: (e: React.MouseEvent) => void;
  onMouseUp: () => void;
  onMouseLeave: () => void;
  onTouchStart: (e: React.TouchEvent) => void;
  onTouchEnd: () => void;
};

export interface TvPlayer {
  clientNum: number;
  name: string;
  team: number;
  model: string;
  isVR: boolean;
}

export interface UseTvEngineOptions {
  /** Exactly one of demoUrl / liveUrl drives the source. */
  demoUrl?: string;
  liveUrl?: string;
  /** Live only — TVL1 carries no configstrings, so fs_game is explicit. */
  fsGame?: string;
  /** Caller-specific engine args, e.g. "+set cl_demoPlayer 1". */
  extraArgs?: string;
  /** Bump to tear down the current module and re-boot onto the same canvas. */
  reloadKey?: number;
  onReady?: () => void;
  /** Live only: fired once per ready-session when live playback has drained to
   *  the end (cl_tvLiveEnded). Re-armed across a reconnect reboot, so tolerate a
   *  repeat (e.g. a resize after end re-observes the cvar). */
  onStreamClosed?: () => void;
  /** Fired on a canvas touchstart — the page uses it to dismiss open overlays
   *  (e.g. the follow flyout), which the capture-phase handler would otherwise
   *  swallow before the document outside-tap handler could. */
  onCanvasTouchStart?: () => void;
}

export interface UseTvEngine {
  canvasRef: React.RefObject<HTMLCanvasElement | null>;
  statusRef: React.RefObject<HTMLDivElement | null>;
  moduleRef: React.MutableRefObject<EngineModule | null>;
  engineReady: boolean;
  loading: boolean;
  progress: { loaded: number; total: number };
  error: string | null;
  playerList: TvPlayer[];
  viewpoint: number;
  /** Engine-reported current/target live map (cl_tvMapName). Drives the levelshot
   *  backdrop, and crosses an in-place map change without the /api/servers lag. */
  liveMapName: string;
  /** True from the start of an in-place live map change (cl_tvMapName changed)
   *  until it completes (cl_tvMapSerial bumped) — gates the transition overlay. */
  transitioning: boolean;
  refreshPlayerList: () => void;
  follow: (clientNum: number) => void;
  runCmd: (cmd: string) => void;
  readCvarNumber: (name: string) => number;
  sendKey: SendKey;
  sendMouse: (button: number, type: "mousedown" | "mouseup") => void;
  preventFocus: (e: React.MouseEvent) => void;
  holdHandlers: HoldHandlers;
  /** Scrub mode = a held ShiftLeft modifier the touch handler must finalize, so
   *  the hook owns it; the page renders the toggle off scrubActive. */
  scrubActive: boolean;
  setScrub: (next: boolean) => void;
}

// The console font is sized in device pixels (8px * con_scale), so on a large
// or hi-DPI framebuffer it renders tiny. Scale it to the classic 640x480 grid,
// picking the SMALLER of the width/height ratios so an extreme aspect (ultrawide
// monitor, tall phone) doesn't oversize the font by its larger dimension. The
// engine clamps con_scale to [0.5, 8]; clamp here too for a tidy value.
function consoleScale(fbWidth: number, fbHeight: number): number {
  const s = Math.min(fbWidth / 640, fbHeight / 480);
  return Math.round(Math.max(0.5, Math.min(8, s)) * 100) / 100;
}

export function useTvEngine(opts: UseTvEngineOptions): UseTvEngine {
  const {
    demoUrl,
    liveUrl,
    fsGame,
    extraArgs,
    reloadKey,
    onReady,
    onStreamClosed,
    onCanvasTouchStart,
  } = opts;
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const statusRef = useRef<HTMLDivElement>(null);
  const moduleRef = useRef<EngineModule | null>(null);
  const [engineReady, setEngineReady] = useState(false);
  const [loading, setLoading] = useState(true);
  const [progress, setProgress] = useState({ loaded: 0, total: 0 });
  const [error, setError] = useState<string | null>(null);
  const [playerList, setPlayerList] = useState<TvPlayer[]>([]);
  const [viewpoint, setViewpoint] = useState(-1);
  const [liveMapName, setLiveMapName] = useState("");
  const [transitioning, setTransitioning] = useState(false);
  const [scrubActive, setScrubActive] = useState(false);
  const scrubRef = useRef(false);

  // Latest callbacks via refs so they don't retrigger the boot effect; assigned
  // in an effect, not during render.
  const onReadyRef = useRef(onReady);
  const onStreamClosedRef = useRef(onStreamClosed);
  const onCanvasTouchStartRef = useRef(onCanvasTouchStart);
  useEffect(() => {
    onReadyRef.current = onReady;
    onStreamClosedRef.current = onStreamClosed;
    onCanvasTouchStartRef.current = onCanvasTouchStart;
  });

  const runCmd = useCallback((cmd: string) => {
    const mod = moduleRef.current;
    if (!mod?.ccall) return;
    try {
      mod.ccall(
        "Cbuf_AddText",
        null,
        ["string"],
        [cmd.endsWith("\n") ? cmd : cmd + "\n"],
      );
    } catch (e) {
      console.debug("runCmd failed", cmd, e);
    }
  }, []);

  const readCvarNumber = useCallback((name: string) => {
    const mod = moduleRef.current;
    if (!mod?.ccall) return 0;
    try {
      return mod.ccall("Cvar_VariableValue", "number", ["string"], [name]);
    } catch (e) {
      console.debug("readCvarNumber failed", name, e);
      return 0;
    }
  }, []);

  const readCvarString = useCallback((name: string) => {
    const mod = moduleRef.current;
    if (!mod?.ccall) return "";
    try {
      return (
        mod.ccall("Cvar_VariableString", "string", ["string"], [name]) || ""
      );
    } catch (e) {
      console.debug("readCvarString failed", name, e);
      return "";
    }
  }, []);

  const refreshPlayerList = useCallback(() => {
    const mod = moduleRef.current;
    if (!mod?.ccall) return;
    try {
      const raw = mod.ccall("CL_TV_GetPlayerList", "string", [], []);
      if (!raw) return;
      const lines = raw.split("\n").filter(Boolean);
      if (lines.length < 1) return;
      setViewpoint(parseInt(lines[0], 10));
      const players = lines.slice(1).map((line) => {
        const [num, name, team, model, vr] = line.split("\t");
        return {
          clientNum: parseInt(num, 10),
          name: name || "",
          team: parseInt(team, 10),
          model: model || "",
          isVR: vr === "1",
        };
      });
      players.sort((a, b) => a.team - b.team || a.clientNum - b.clientNum);
      setPlayerList(players);
    } catch (e) {
      console.debug("refreshPlayerList failed", e);
    }
  }, []);

  const follow = useCallback(
    (clientNum: number) => {
      runCmd(`tv_view ${clientNum}`);
      setViewpoint(clientNum);
    },
    [runCmd],
  );

  // Scrub = a held ShiftLeft; SDL2 listens on document, so dispatch there.
  const setScrub = useCallback((next: boolean) => {
    scrubRef.current = next;
    setScrubActive(next);
    document.dispatchEvent(
      new KeyboardEvent(next ? "keydown" : "keyup", {
        code: "ShiftLeft",
        key: "ShiftLeft",
        bubbles: true,
      }),
    );
  }, []);

  // Boot / re-boot the engine; reloadKey in deps lets live reconnect by tearing
  // down and re-opening onto the same canvas.
  useEffect(() => {
    if (!demoUrl && !liveUrl) return;
    let aborted = false;

    async function init() {
      // Reset here (not in the effect body) to avoid a synchronous setState
      // mid-effect. Roster resets too: on a live reboot (map swap) the follow
      // flyout would otherwise show the previous match's players until the next
      // hover refresh.
      setLoading(true);
      setError(null);
      setEngineReady(false);
      setPlayerList([]);
      setViewpoint(-1);
      try {
        const { loadEngine } = await import("/engine/loader.js");
        if (aborted || !canvasRef.current || !statusRef.current) return;
        const rect = canvasRef.current.getBoundingClientRect();
        const dpr = window.devicePixelRatio || 1;
        const fbW = Math.round(rect.width * dpr);
        const fbH = Math.round(rect.height * dpr);
        const sizeArgs = `+set r_mode -1 +set r_customwidth ${fbW} +set r_customheight ${fbH} +set con_scale ${consoleScale(fbW, fbH)}`;
        const mod = await loadEngine({
          canvas: canvasRef.current,
          statusEl: statusRef.current,
          enginePath: "/engine/",
          configUrl: "/engine/demo-config.json",
          demoUrl,
          liveUrl,
          fsGame,
          extraArgs: `${extraArgs ?? ""} ${sizeArgs}`.trim(),
          onProgress: (loaded, total) => setProgress({ loaded, total }),
          onReady: () => {
            setEngineReady(true);
            if (statusRef.current) statusRef.current.style.display = "none";
            onReadyRef.current?.();
          },
        });
        moduleRef.current = mod;
        if (aborted) {
          try {
            mod.abort();
          } catch {
            /* engine may not be ready */
          }
          return;
        }
        setLoading(false);
      } catch (e) {
        if (!aborted)
          setError(e instanceof Error ? e.message : "Failed to load");
      }
    }
    init();

    return () => {
      aborted = true;
      const mod = moduleRef.current;
      if (mod) {
        try {
          mod.shutdown();
        } catch (e) {
          console.debug("shutdown failed", e);
        }
        try {
          mod.pauseMainLoop();
        } catch (e) {
          console.debug("pauseMainLoop failed", e);
        }
        try {
          mod._exit(0);
        } catch (e) {
          console.debug("_exit failed", e);
        }
        moduleRef.current = null;
      }
    };
  }, [demoUrl, liveUrl, fsGame, extraArgs, reloadKey]);

  // Live: poll cl_tvLiveEnded (playback drained), not cl_tvStreamClosed (fetch
  // closed ~delay-buffer earlier), so reconnect waits for the last seconds.
  useEffect(() => {
    if (!liveUrl || !engineReady) return;
    let fired = false;
    const iv = setInterval(() => {
      if (fired) return;
      if (readCvarNumber("cl_tvLiveEnded") >= 1) {
        fired = true;
        onStreamClosedRef.current?.();
      }
    }, 500);
    return () => clearInterval(iv);
  }, [liveUrl, engineReady, readCvarNumber]);

  // Live: cross an in-place map change without a reload. The engine sets
  // cl_tvMapName at the START of a change (raise the levelshot overlay covering the
  // pk3 download) and bumps cl_tvMapSerial at the END (refresh the roster). See
  // cl_tv.c CL_TV_LiveMapChange. The initial map (first read) seeds the name
  // without flagging a transition — the boot loading state already covers it.
  useEffect(() => {
    if (!liveUrl || !engineReady) return;
    let prevName: string | null = null;
    let prevSerial = readCvarNumber("cl_tvMapSerial");
    const iv = setInterval(() => {
      const name = readCvarString("cl_tvMapName");
      if (name && name !== prevName) {
        if (prevName !== null) setTransitioning(true);
        setLiveMapName(name);
        prevName = name;
      }
      const serial = readCvarNumber("cl_tvMapSerial");
      if (serial !== prevSerial) {
        prevSerial = serial;
        setTransitioning(false);
        refreshPlayerList();
      }
    }, 250);
    return () => clearInterval(iv);
  }, [liveUrl, engineReady, readCvarNumber, readCvarString, refreshPlayerList]);

  // Re-initialize video on resize so the framebuffer matches the CSS box
  useEffect(() => {
    const canvas = canvasRef.current;
    const mod = moduleRef.current;
    if (!canvas || !mod) return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const dpr = window.devicePixelRatio || 1;
    let initW = Math.round(canvas.getBoundingClientRect().width * dpr);
    let initH = Math.round(canvas.getBoundingClientRect().height * dpr);
    const observer = new ResizeObserver(() => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        const rect = canvas.getBoundingClientRect();
        const w = Math.round(rect.width * dpr);
        const h = Math.round(rect.height * dpr);
        if (w === initW && h === initH) return;
        initW = w;
        initH = h;
        flushSync(() => setEngineReady(false));
        canvas.style.visibility = "hidden";
        if (statusRef.current) {
          statusRef.current.style.display = "";
          statusRef.current.textContent = "Restarting video...";
        }
        // Defer vid_restart so browser paints the overlay first
        requestAnimationFrame(() => {
          mod.ccall(
            "Cbuf_AddText",
            null,
            ["string"],
            [
              `r_mode -1\nr_customwidth ${w}\nr_customheight ${h}\ncon_scale ${consoleScale(w, h)}\nvid_restart\n`,
            ],
          );
          // Schedule after Cbuf_AddText so the reveal fires on the engine frame
          // AFTER vid_restart (our rAF runs after the engine's, so the next
          // postMainLoop is post-restart).
          mod.onNextFrame?.(() => {
            canvas.style.visibility = "";
            setEngineReady(true);
            if (statusRef.current) statusRef.current.style.display = "none";
          });
        });
      }, 200);
    });
    observer.observe(canvas);
    return () => {
      observer.disconnect();
      if (timer) clearTimeout(timer);
    };
  }, [loading]);

  // Intercept touch before SDL2 (capture phase) so touches rotate the camera
  // but don't click (follow next).
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    let lastX = 0,
      lastY = 0;
    // SDL computes deltas from absolute coords, so track a virtual cursor moved
    // only by touch deltas — avoids a camera jump on first touch.
    const rect = canvas.getBoundingClientRect();
    let synthX = rect.left + rect.width / 2;
    let synthY = rect.top + rect.height / 2;
    let pinchDist = 0;
    const PINCH_STEP = 30; // pixels of pinch distance per zoom step
    const pinchLen = (t: TouchList) => {
      const dx = t[0].clientX - t[1].clientX;
      const dy = t[0].clientY - t[1].clientY;
      return Math.sqrt(dx * dx + dy * dy);
    };
    const onStart = (e: TouchEvent) => {
      e.stopImmediatePropagation();
      e.preventDefault();
      // stopImmediatePropagation above blocks the document outside-tap handler,
      // so dismiss overlays explicitly.
      onCanvasTouchStartRef.current?.();
      const ct = e.targetTouches;
      if (ct.length >= 2) {
        pinchDist = pinchLen(ct);
      } else if (ct.length === 1) {
        lastX = ct[0].clientX;
        lastY = ct[0].clientY;
        // Zero sensitivity while resetting the synthetic cursor to the touch
        // point so the position jump is invisible; restore after one frame.
        const mod = moduleRef.current;
        if (mod?.ccall) {
          let sens = "5";
          try {
            const v = mod.ccall(
              "Cvar_VariableString",
              "string",
              ["string"],
              ["sensitivity"],
            );
            if (v) sens = v;
          } catch (e) {
            console.debug("Cvar_VariableString failed", e);
          }
          mod.ccall("Cbuf_AddText", null, ["string"], ["sensitivity 0\n"]);
          canvas.dispatchEvent(
            new MouseEvent("mousemove", {
              clientX: lastX,
              clientY: lastY,
              bubbles: true,
            }),
          );
          synthX = lastX;
          synthY = lastY;
          setTimeout(() => {
            try {
              mod.ccall(
                "Cbuf_AddText",
                null,
                ["string"],
                [`sensitivity ${sens}\n`],
              );
            } catch (e) {
              console.debug("sensitivity restore failed", e);
            }
          }, 50);
        }
      }
    };
    const onMove = (e: TouchEvent) => {
      e.stopImmediatePropagation();
      e.preventDefault();
      const ct = e.targetTouches;
      if (ct.length >= 2) {
        const dist = pinchLen(ct);
        const steps = Math.trunc((dist - pinchDist) / PINCH_STEP);
        if (steps !== 0) {
          for (let i = 0; i < Math.abs(steps); i++)
            canvas.dispatchEvent(
              new WheelEvent("wheel", {
                deltaY: steps > 0 ? -120 : 120,
                bubbles: true,
              }),
            );
          pinchDist += steps * PINCH_STEP;
        }
      } else if (ct.length === 1) {
        const t = ct[0];
        const dx = t.clientX - lastX;
        const dy = t.clientY - lastY;
        synthX += dx;
        synthY += dy;
        // Clamp to canvas bounds — SDL clamps internally, so if we don't
        // match, our position diverges and all deltas become zero.
        const b = canvas.getBoundingClientRect();
        synthX = Math.max(b.left, Math.min(b.right, synthX));
        synthY = Math.max(b.top, Math.min(b.bottom, synthY));
        canvas.dispatchEvent(
          new MouseEvent("mousemove", {
            clientX: synthX,
            clientY: synthY,
            movementX: dx,
            movementY: dy,
            bubbles: true,
          }),
        );
        lastX = t.clientX;
        lastY = t.clientY;
      }
    };
    const onEnd = (e: TouchEvent) => {
      e.stopImmediatePropagation();
      e.preventDefault();
      if (scrubRef.current) {
        canvas.dispatchEvent(
          new MouseEvent("mousedown", {
            clientX: lastX,
            clientY: lastY,
            button: 0,
            bubbles: true,
          }),
        );
        canvas.dispatchEvent(
          new MouseEvent("mouseup", {
            clientX: lastX,
            clientY: lastY,
            button: 0,
            bubbles: true,
          }),
        );
        scrubRef.current = false;
        setScrubActive(false);
        document.dispatchEvent(
          new KeyboardEvent("keyup", {
            code: "ShiftLeft",
            key: "ShiftLeft",
            bubbles: true,
          }),
        );
      }
    };
    const onMouseUp = () => {
      if (scrubRef.current) {
        scrubRef.current = false;
        setScrubActive(false);
        document.dispatchEvent(
          new KeyboardEvent("keyup", {
            code: "ShiftLeft",
            key: "ShiftLeft",
            bubbles: true,
          }),
        );
      }
    };
    canvas.addEventListener("touchstart", onStart, true);
    canvas.addEventListener("touchmove", onMove, true);
    canvas.addEventListener("touchend", onEnd, true);
    canvas.addEventListener("mouseup", onMouseUp);
    return () => {
      canvas.removeEventListener("touchstart", onStart, true);
      canvas.removeEventListener("touchmove", onMove, true);
      canvas.removeEventListener("touchend", onEnd, true);
      canvas.removeEventListener("mouseup", onMouseUp);
    };
  }, []);

  useEffect(() => {
    const handler = () => {
      canvasRef.current?.classList.toggle(
        "no-pointerlock",
        !document.pointerLockElement,
      );
    };
    document.addEventListener("pointerlockchange", handler);
    return () => document.removeEventListener("pointerlockchange", handler);
  }, []);

  // Prevent Tab key from leaving the canvas
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Tab") e.preventDefault();
    };
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, []);

  // SDL2/Emscripten registers keyboard events on document, not canvas.
  const sendKey = useCallback<SendKey>((code, type) => {
    document.dispatchEvent(
      new KeyboardEvent(type, { code, key: code, bubbles: true }),
    );
  }, []);

  const sendMouse = useCallback(
    (button: number, type: "mousedown" | "mouseup") => {
      canvasRef.current?.dispatchEvent(
        new MouseEvent(type, { button, bubbles: true }),
      );
    },
    [],
  );

  const preventFocus = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
  }, []);

  // Hold-button helpers — preventDefault keeps focus on the canvas.
  const holdHandlers = useCallback<HoldHandlers>(
    (downFn, upFn) => ({
      onMouseDown: (e) => {
        e.preventDefault();
        downFn();
      },
      onMouseUp: () => upFn(),
      onMouseLeave: () => upFn(),
      onTouchStart: (e) => {
        e.preventDefault();
        downFn();
      },
      onTouchEnd: () => upFn(),
    }),
    [],
  );

  return {
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
    refreshPlayerList,
    follow,
    runCmd,
    readCvarNumber,
    sendKey,
    sendMouse,
    preventFocus,
    holdHandlers,
    scrubActive,
    setScrub,
  };
}
