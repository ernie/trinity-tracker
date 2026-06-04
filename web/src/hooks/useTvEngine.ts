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
  /** Deep-link initial view, applied once after the first snapshot arrives — the
   *  tv_view target table is empty until then. initialFollow = client slot to
   *  POV (live: the leader at click time; VOD: the shared ?f=). initialSeek =
   *  VOD seek seconds; ignored for live (no seek in a live stream). */
  initialFollow?: number | null;
  initialSeek?: number | null;
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
  /** False until the initialFollow/initialSeek deep-link has been applied (true
   *  immediately when there is none). Pages keep the levelshot up while false so
   *  the engine's default viewpoint never flashes before the requested POV. */
  initialViewApplied: boolean;
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

// Supported framebuffer aspect ratios (ascending). Resizes within a ratio are
// pure CSS (object-fit: contain letter/pillarboxes the fixed-aspect buffer); a
// vid_restart only fires when the resize crosses to a different ratio or outgrows
// the framebuffer resolution. The game view is landscape, so there's no portrait
// ratio — a portrait container gets 4:3, letterboxed.
const RATIOS = [4 / 3, 16 / 9, 21 / 9, 32 / 9];
const HYST = 0.08; // narrower-switch deadband, so we don't flap at a boundary
const TIER_FACTOR = 1.4; // resolution-reinit deadband (implicit crispness tiers)
const MAX_FB = { w: 3840, h: 2160 };

// Pick the widest ratio R with R <= C (else the narrowest) — prefers pillarbox
// over letterbox, filling width while keeping full height. `current` enables
// anti-flap hysteresis: widening adopts the base pick immediately (the
// widest-R<=C rule already gates it on C reaching that ratio), but narrowing only
// switches once C drops a clear margin below the held ratio.
function selectAspect(
  containerAspect: number,
  current?: number | null,
): number {
  const fits = RATIOS.filter((r) => r <= containerAspect);
  const base = fits.length ? Math.max(...fits) : Math.min(...RATIOS);
  if (current == null || base > current) return base;
  if (base < current)
    return containerAspect < current * (1 - HYST) ? base : current;
  return current;
}

// Framebuffer dims for the displayed content (not the whole box, so no pixels go
// to the bars): height-limited when pillarboxed, width-limited when letterboxed.
// Scaled by DPR and capped to MAX_FB while preserving aspect.
function targetFramebuffer(
  ratio: number,
  boxW: number,
  boxH: number,
  dpr: number,
): { w: number; h: number } {
  const c = boxW / boxH;
  const w = ratio <= c ? boxH * ratio : boxW;
  const h = ratio <= c ? boxH : boxW / ratio;
  let fbW = Math.round(w * dpr);
  let fbH = Math.round(h * dpr);
  if (fbW > MAX_FB.w || fbH > MAX_FB.h) {
    const s = Math.min(MAX_FB.w / fbW, MAX_FB.h / fbH);
    fbW = Math.round(fbW * s);
    fbH = Math.round(fbH * s);
  }
  return { w: fbW, h: fbH };
}

// True when a new target stays close enough to the current framebuffer to just
// rescale via CSS — both dimensions within the tier deadband [1/F, F].
function withinTier(
  target: { w: number; h: number },
  cur: { w: number; h: number },
): boolean {
  const rw = target.w / cur.w;
  const rh = target.h / cur.h;
  const lo = 1 / TIER_FACTOR;
  return rw >= lo && rw <= TIER_FACTOR && rh >= lo && rh <= TIER_FACTOR;
}

export function useTvEngine(opts: UseTvEngineOptions): UseTvEngine {
  const {
    demoUrl,
    liveUrl,
    fsGame,
    extraArgs,
    initialFollow,
    initialSeek,
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
  // Init true when there's no deep-link to apply, so pages reveal immediately.
  const [initialViewApplied, setInitialViewApplied] = useState(
    () => initialFollow == null && initialSeek == null,
  );
  const [scrubActive, setScrubActive] = useState(false);
  const scrubRef = useRef(false);
  // Current aspect bucket + framebuffer, tracked in refs (not state) so resize
  // ticks don't churn renders. Seeded at boot, updated only on an actual reinit.
  const currentRatioRef = useRef<number | null>(null);
  const currentFBRef = useRef<{ w: number; h: number } | null>(null);

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
        // Seed the framebuffer through the aspect selector so first paint already
        // uses a supported ratio (not the raw box), matching the resize path.
        const ratio = selectAspect(rect.width / rect.height);
        const fb = targetFramebuffer(ratio, rect.width, rect.height, dpr);
        currentRatioRef.current = ratio;
        currentFBRef.current = fb;
        const sizeArgs = `+set r_mode -1 +set r_customwidth ${fb.w} +set r_customheight ${fb.h} +set con_scale ${consoleScale(fb.w, fb.h)}`;
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

  // Apply the initialFollow/initialSeek deep-link once the engine is live AND a
  // snapshot has arrived — tv_view/tv_seek need the player table populated to
  // validate the target, and there's no JS-side "first snap" hook, so we poll
  // CL_TV_GetPlayerList (empty until the first snapshot) until it lists a player.
  // Once-ever per mount (the ref guard survives the engineReady cycling a resize
  // causes); a live reconnect is a full document reload, so it re-arms there.
  const initialViewAppliedRef = useRef(false);
  useEffect(() => {
    if (!engineReady || initialViewAppliedRef.current) return;
    if (initialFollow == null && initialSeek == null) {
      initialViewAppliedRef.current = true;
      return;
    }
    const mod = moduleRef.current;
    if (!mod?.ccall) return;
    const iv = setInterval(() => {
      let raw: string | null = null;
      try {
        raw = mod.ccall("CL_TV_GetPlayerList", "string", [], []);
      } catch (e) {
        console.debug("CL_TV_GetPlayerList failed", e);
        return;
      }
      if (!raw) return;
      // header line + at least one player ⇒ the target table is valid.
      if (raw.split("\n").filter(Boolean).length < 2) return;
      initialViewAppliedRef.current = true;
      clearInterval(iv);
      if (initialFollow != null) follow(initialFollow);
      if (initialSeek != null) runCmd(`tv_seek ${initialSeek}`); // VOD-only
      // One engine frame so the cbuf runs tv_view/tv_seek before the page drops
      // the levelshot and reveals the canvas at the requested POV.
      mod.onNextFrame?.(() => setInitialViewApplied(true));
    }, 100);
    return () => clearInterval(iv);
  }, [engineReady, initialFollow, initialSeek, follow, runCmd]);

  // On resize, quantize the framebuffer to a supported aspect bucket and let CSS
  // (object-fit: contain) scale within it — so dragging, the mobile URL bar, and
  // minor reflow are pure CSS, no engine call. A vid_restart fires only when the
  // resize crosses to a different aspect bucket or outgrows/undergrows the
  // framebuffer resolution past the tier deadband.
  useEffect(() => {
    const canvas = canvasRef.current;
    const mod = moduleRef.current;
    if (!canvas || !mod) return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const settle = () => {
      const rect = canvas.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) return;
      const dpr = window.devicePixelRatio || 1; // re-read: monitor moves change it
      const prevRatio = currentRatioRef.current;
      const ratio = selectAspect(rect.width / rect.height, prevRatio);
      const target = targetFramebuffer(ratio, rect.width, rect.height, dpr);
      const cur = currentFBRef.current;
      const bucketChanged = ratio !== prevRatio;
      if (!bucketChanged && cur && withinTier(target, cur)) return; // CSS handles it
      currentRatioRef.current = ratio;
      currentFBRef.current = target;
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
            `r_mode -1\nr_customwidth ${target.w}\nr_customheight ${target.h}\ncon_scale ${consoleScale(target.w, target.h)}\nvid_restart\n`,
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
    };
    const debounced = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(settle, 200);
    };
    const observer = new ResizeObserver(debounced);
    observer.observe(canvas);
    // Phone rotate: object-fit handles the box reflow, but the bucket changes
    // (portrait 4:3 → landscape 16:9), so route it through the same settle path.
    window.addEventListener("orientationchange", debounced);
    return () => {
      observer.disconnect();
      window.removeEventListener("orientationchange", debounced);
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
    initialViewApplied,
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
