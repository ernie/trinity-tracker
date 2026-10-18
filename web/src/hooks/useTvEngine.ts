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
  /** Live only: a TVL1 stream carries no configstrings, so fs_game is passed. */
  fsGame?: string;
  /** Extra engine args, such as "+set cl_demoPlayer 1". */
  extraArgs?: string;
  /** The deep-linked first view, applied once after the first snapshot, when
   *  tv_view's target table fills. initialFollow is the client slot to follow
   *  (live: the leader at click time; VOD: the shared ?f=). initialSeek is the
   *  VOD seek in seconds; live streams ignore it. */
  initialFollow?: number | null;
  initialSeek?: number | null;
  /** Bump to tear down the current module and re-boot onto the same canvas. */
  reloadKey?: number;
  onReady?: () => void;
  /** Live only: fires once per ready session when playback has drained to the
   *  end (cl_tvLiveEnded). It can repeat, since a reboot or a resize after the
   *  end re-arms it. */
  onStreamClosed?: () => void;
  /** Fires on a canvas touchstart so the page can close overlays such as the
   *  follow flyout; the capture-phase touch handler hides the tap from the
   *  document. */
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
  /** The live map the engine reports (cl_tvMapName), for the levelshot backdrop.
   *  It follows an in-place map change sooner than /api/servers. */
  liveMapName: string;
  /** True from the start of an in-place live map change (cl_tvMapName changes)
   *  until it completes (cl_tvMapSerial bumps); shows the transition overlay. */
  transitioning: boolean;
  /** False until the initialFollow/initialSeek deep link is applied (true at
   *  once with none). Pages keep the levelshot up meanwhile, so the engine's
   *  default view never flashes before the requested one. */
  initialViewApplied: boolean;
  refreshPlayerList: () => void;
  follow: (clientNum: number) => void;
  runCmd: (cmd: string) => void;
  readCvarNumber: (name: string) => number;
  sendKey: SendKey;
  sendMouse: (button: number, type: "mousedown" | "mouseup") => void;
  preventFocus: (e: React.MouseEvent) => void;
  holdHandlers: HoldHandlers;
  /** Scrub mode is a held ShiftLeft that the touch handler releases, so the
   *  hook owns it; the page draws its toggle from scrubActive. */
  scrubActive: boolean;
  setScrub: (next: boolean) => void;
}

// The console font is 8px * con_scale in device pixels, tiny on a large
// framebuffer. Scale to the 640x480 grid by the smaller ratio, so an ultrawide
// or tall screen keeps a sensible size. The engine clamps con_scale to [0.5, 8].
function consoleScale(fbWidth: number, fbHeight: number): number {
  const s = Math.min(fbWidth / 640, fbHeight / 480);
  return Math.round(Math.max(0.5, Math.min(8, s)) * 100) / 100;
}

// Framebuffer aspect ratios, ascending. The game view is landscape, so a
// portrait container gets 4:3, letterboxed.
const RATIOS = [4 / 3, 16 / 9, 21 / 9, 32 / 9];
const HYST = 0.08; // deadband before switching to a narrower ratio
const TIER_FACTOR = 1.4; // deadband before a resolution change restarts video
const MAX_FB = { w: 3840, h: 2160 };

// The widest ratio R <= C, else the narrowest, so the view fills the width and
// keeps its full height. With `current`, a wider pick applies at once, while a
// narrower one waits until C falls a clear margin below the held ratio.
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

// Sized to the displayed content so no pixels go to the bars: height-limited
// when pillarboxed, width-limited when letterboxed. Scaled by DPR and capped to
// MAX_FB at the same aspect.
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

// True when both dimensions stay within [1/F, F] of the current framebuffer,
// so CSS can rescale it.
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
  const [initialViewApplied, setInitialViewApplied] = useState(
    () => initialFollow == null && initialSeek == null,
  );
  const [scrubActive, setScrubActive] = useState(false);
  const scrubRef = useRef(false);
  // Refs so resize ticks don't re-render; set at boot and on each restart.
  const currentRatioRef = useRef<number | null>(null);
  const currentFBRef = useRef<{ w: number; h: number } | null>(null);

  // Refs so new callbacks don't rerun the boot effect.
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

  // reloadKey lets a live reconnect tear down and reboot onto the same canvas.
  useEffect(() => {
    if (!demoUrl && !liveUrl) return;
    let aborted = false;

    async function init() {
      // Reset inside init to keep setState out of the effect body. The roster
      // resets too, so a reboot's follow flyout starts empty.
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
        // First paint goes through the aspect selector, as resizes do.
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
          setError(
            e instanceof Error ? e.message : "Couldn't load the viewer.",
          );
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

  // Live: cl_tvLiveEnded means playback has drained, so the reconnect waits
  // out the last seconds of the delay buffer.
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

  // Live: in-place map changes (cl_tv.c CL_TV_LiveMapChange). The engine sets
  // cl_tvMapName when a change starts, which raises the levelshot over the pk3
  // download, and bumps cl_tvMapSerial when it ends, which refreshes the roster.
  // The first read only seeds the name; the boot loading state covers it.
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

  // tv_view and tv_seek need the player table, which fills with the first
  // snapshot, so poll CL_TV_GetPlayerList until it lists a player. Applied once
  // per mount: the ref outlasts the engineReady cycle a resize causes, and a
  // live reconnect reloads the page.
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
      // A header line plus at least one player.
      if (raw.split("\n").filter(Boolean).length < 2) return;
      initialViewAppliedRef.current = true;
      clearInterval(iv);
      if (initialFollow != null) follow(initialFollow);
      if (initialSeek != null) runCmd(`tv_seek ${initialSeek}`);
      // Wait one engine frame so the commands run before the canvas shows.
      mod.onNextFrame?.(() => setInitialViewApplied(true));
    }, 100);
    return () => clearInterval(iv);
  }, [engineReady, initialFollow, initialSeek, follow, runCmd]);

  // A background tab's timers are throttled, so the engine stalls while the
  // relay keeps filling its buffer. On return, tv_resync jumps back to the live
  // cushion. The engine ignores it for VOD and when nothing is buffered.
  useEffect(() => {
    if (!engineReady) return;
    let hiddenAt = 0;
    const onVisibility = () => {
      if (document.hidden) {
        hiddenAt = performance.now();
        return;
      }
      // Quick tab flips leave too little backlog to matter.
      if (hiddenAt && performance.now() - hiddenAt > 1000) runCmd("tv_resync");
      hiddenAt = 0;
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, [engineReady, runCmd]);

  // CSS (object-fit: contain) handles resizes within an aspect bucket and tier,
  // such as a drag or the mobile URL bar. Crossing to another bucket or past the
  // tier deadband restarts video at the new size.
  useEffect(() => {
    const canvas = canvasRef.current;
    const mod = moduleRef.current;
    if (!canvas || !mod) return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    // After a rotation and restart, iOS Safari's taps land about a toolbar
    // height below the finger until a drag reconciles its viewports; this does
    // the drag's work. It takes both this and the settle delay below.
    const reconcileViewport = () => {
      window.scrollTo(0, 0);
      const page = canvas.closest(".demo-player-page");
      if (page instanceof HTMLElement) {
        page.style.transform = "translateZ(0)";
        page.getBoundingClientRect();
        page.style.transform = "";
      }
    };
    const settle = () => {
      reconcileViewport();
      const rect = canvas.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) return;
      const dpr = window.devicePixelRatio || 1; // changes when moved to another monitor
      const prevRatio = currentRatioRef.current;
      const ratio = selectAspect(rect.width / rect.height, prevRatio);
      const target = targetFramebuffer(ratio, rect.width, rect.height, dpr);
      const cur = currentFBRef.current;
      const bucketChanged = ratio !== prevRatio;
      if (!bucketChanged && cur && withinTier(target, cur)) return;
      currentRatioRef.current = ratio;
      currentFBRef.current = target;
      flushSync(() => setEngineReady(false));
      canvas.style.visibility = "hidden";
      if (statusRef.current) {
        statusRef.current.style.display = "";
        statusRef.current.textContent = "Restarting video…";
      }
      // Deferred so the overlay paints first.
      requestAnimationFrame(() => {
        mod.ccall(
          "Cbuf_AddText",
          null,
          ["string"],
          [
            `r_mode -1\nr_customwidth ${target.w}\nr_customheight ${target.h}\ncon_scale ${consoleScale(target.w, target.h)}\nvid_restart\n`,
          ],
        );
        // This rAF runs after the engine's, so the next engine frame is the
        // first one after vid_restart.
        mod.onNextFrame?.(() => {
          canvas.style.visibility = "";
          setEngineReady(true);
          if (statusRef.current) statusRef.current.style.display = "none";
          reconcileViewport();
        });
      });
    };
    // Waits well past the rotation animation: a vid_restart blocks the main
    // thread, and one during iOS Safari's rotation settle offsets its taps.
    const debounced = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(settle, 1000);
    };
    const observer = new ResizeObserver(debounced);
    observer.observe(canvas);
    // A rotation changes the aspect bucket.
    window.addEventListener("orientationchange", debounced);
    return () => {
      observer.disconnect();
      window.removeEventListener("orientationchange", debounced);
      if (timer) clearTimeout(timer);
    };
  }, [loading]);

  // Capture phase, ahead of SDL2, so a touch turns the camera; an SDL click
  // would follow the next player.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    let lastX = 0,
      lastY = 0;
    // SDL derives deltas from absolute positions, so a virtual cursor moved only
    // by touch deltas keeps the first touch from jumping the camera.
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
        // SDL clamps to the canvas too; past the edge the two would disagree and
        // every delta would come out zero.
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

  // Tab would move focus off the canvas.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Tab") e.preventDefault();
    };
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, []);

  // SDL2 listens for keys on the document.
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

  // preventDefault keeps focus on the canvas.
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
