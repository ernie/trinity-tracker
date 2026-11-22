import * as THREE from "three";
import { HeadScene } from "./headScene";
import { HeadIdleAnimator } from "./headIdleAngles";
import {
  frameIntervalMs,
  HEAD_OVERFLOW,
  phaseOffsetFor,
} from "./headRenderMath";
import { HonkClock, honkPose } from "./headHonk";
import { playHonk } from "./honkSound";

export interface HeadHandle {
  release(): void;
  honk(): void;
}

// Square render resolution per head; each head's target canvas is sized to
// match and CSS-scaled to its box.
const RENDER_PX = 256;

// The honk overlay spans this many head boxes, matching ModelHead.css, at the
// head canvas's pixel density.
const HONK_OVERLAY_BOXES = 4;
const OVERLAY_PX = Math.round((RENDER_PX * HONK_OVERLAY_BOXES) / HEAD_OVERFLOW);

interface HeadEntry {
  canvas: HTMLCanvasElement;
  ctx: CanvasRenderingContext2D;
  model: string;
  skin: string;
  scene: HeadScene | null;
  animator: HeadIdleAnimator;
  phaseOffset: number;
  visible: boolean;
  // Set the first time the head scrolls into view; load is deferred until then.
  loadStarted: boolean;
  loaded: boolean;
  // Bumped on context loss so a load in flight at that moment can't install a
  // scene built against the dead context.
  loadGen: number;
  onLive: () => void;
  overlay: CanvasRenderingContext2D | null;
  honkClock: HonkClock | null;
  honking: boolean;
}

const matches = (q: string) =>
  typeof window !== "undefined" && !!window.matchMedia?.(q).matches;

class SharedHeadRenderer {
  private renderer: THREE.WebGLRenderer | null = null;
  private glCanvas: HTMLCanvasElement | null = null;
  private supported = true;
  private entries = new Set<HeadEntry>();
  private io: IntersectionObserver | null = null;
  private raf = 0;
  private lastFrame = 0;
  private index = 0;
  private coarse = matches("(pointer: coarse)");
  private reducedMotion = matches("(prefers-reduced-motion: reduce)");
  private boundFrame = (t: number) => this.frame(t);
  private boundWake = () => this.wake();

  register(
    canvas: HTMLCanvasElement,
    model: string,
    skin: string,
    onLive: () => void,
    overlayCanvas: HTMLCanvasElement | null = null,
  ): HeadHandle | null {
    if (!this.ensureContext()) return null;
    const ctx = canvas.getContext("2d");
    if (!ctx) return null;
    const entry: HeadEntry = {
      canvas,
      ctx,
      model,
      skin,
      scene: null,
      animator: new HeadIdleAnimator(),
      phaseOffset: phaseOffsetFor(this.index++),
      visible: false,
      loadStarted: false,
      loaded: false,
      loadGen: 0,
      onLive,
      overlay: overlayCanvas?.getContext("2d") ?? null,
      honkClock: null,
      honking: false,
    };
    this.entries.add(entry);
    this.io!.observe(canvas);
    return {
      release: () => this.release(entry),
      honk: () => this.honk(entry),
    };
  }

  private honk(entry: HeadEntry): void {
    const honk = entry.scene?.honk;
    if (this.reducedMotion || !entry.loaded || !honk || !entry.overlay) return;
    entry.honkClock ??= new HonkClock(honk.durationMs);
    if (!entry.honkClock.start(performance.now())) return;
    // Most heads never honk, so the large overlay is sized on first use.
    const overlay = entry.overlay.canvas;
    if (overlay.width !== OVERLAY_PX) {
      overlay.width = OVERLAY_PX;
      overlay.height = OVERLAY_PX;
    }
    entry.scene!.resetHonk();
    entry.honking = true;
    if (honk.soundUrl) playHonk(honk.soundUrl);
    this.wake();
  }

  private stopHonk(head: HeadEntry): void {
    if (!head.honking) return;
    head.honking = false;
    head.scene?.setHonkPose(null);
    const o = head.overlay!;
    o.setTransform(1, 0, 0, 1, 0, 0);
    o.clearRect(0, 0, o.canvas.width, o.canvas.height);
  }

  private poseHonk(head: HeadEntry, now: number): void {
    const scene = head.scene!;
    const since = head.honkClock?.since(now) ?? null;
    if (since === null || !scene.honk) {
      this.stopHonk(head);
      return;
    }
    const { frames, fps } = scene.honk.manifest;
    scene.setHonkPose(honkPose(since, frames, fps));
  }

  private async loadEntry(entry: HeadEntry): Promise<void> {
    const gen = entry.loadGen;
    const scene = new HeadScene();
    try {
      await scene.load(entry.model, entry.skin);
    } catch {
      scene.dispose();
      // A missing head bundle is expected; the poster stands in.
      if (entry.loadGen === gen) this.release(entry);
      return;
    }
    // Released, or superseded by a context-loss reload, while we awaited.
    if (!this.entries.has(entry) || entry.loadGen !== gen) {
      scene.dispose();
      return;
    }
    entry.scene?.dispose();
    entry.scene = scene;
    entry.loaded = true;
    entry.onLive();
    this.wake();
  }

  private release(entry: HeadEntry): void {
    if (!this.entries.has(entry)) return;
    this.entries.delete(entry);
    this.io?.unobserve(entry.canvas);
    entry.scene?.dispose();
    entry.scene = null;
  }

  private ensureContext(): boolean {
    if (this.renderer) return true;
    if (!this.supported) return false;
    try {
      const canvas = document.createElement("canvas");
      canvas.width = RENDER_PX;
      canvas.height = RENDER_PX;
      const renderer = new THREE.WebGLRenderer({
        canvas,
        alpha: true,
        antialias: true,
        premultipliedAlpha: false,
        // The buffer is read back via drawImage after each render.
        preserveDrawingBuffer: true,
      });
      renderer.outputColorSpace = THREE.SRGBColorSpace;
      renderer.setClearColor(0x000000, 0);
      canvas.addEventListener("webglcontextlost", this.onContextLost);
      canvas.addEventListener("webglcontextrestored", this.onContextRestored);
      this.io = new IntersectionObserver((es) => this.onIntersect(es), {
        threshold: 0.01,
      });
      document.addEventListener("visibilitychange", this.boundWake);
      this.glCanvas = canvas;
      this.renderer = renderer;
      return true;
    } catch {
      this.supported = false;
      return false;
    }
  }

  private onIntersect(entries: IntersectionObserverEntry[]): void {
    for (const e of entries) {
      for (const head of this.entries) {
        if (head.canvas !== e.target) continue;
        head.visible = e.isIntersecting;
        if (e.isIntersecting && !head.loadStarted) {
          head.loadStarted = true;
          void this.loadEntry(head);
        }
      }
    }
    this.wake();
  }

  private onContextLost = (e: Event) => {
    e.preventDefault();
    cancelAnimationFrame(this.raf);
    this.raf = 0;
    for (const head of this.entries) {
      head.loadGen++;
      head.scene?.dispose();
      head.scene = null;
      head.loaded = false;
      this.stopHonk(head);
    }
  };

  private onContextRestored = () => {
    for (const head of this.entries) {
      if (head.loadStarted) void this.loadEntry(head);
    }
  };

  private wake(): void {
    if (this.raf || !this.renderer) return;
    this.raf = requestAnimationFrame(this.boundFrame);
  }

  private frame(now: number): void {
    this.raf = 0;
    const r = this.renderer;
    const gl = this.glCanvas;
    if (!r || !gl || document.hidden) return;

    const visible: HeadEntry[] = [];
    for (const head of this.entries) {
      if (head.visible && head.scene && head.loaded) visible.push(head);
      else this.stopHonk(head);
    }
    if (visible.length === 0) return; // parked until a wake()

    if (!this.reducedMotion) {
      const interval = frameIntervalMs(visible.length, this.coarse);
      if (now - this.lastFrame < interval) {
        this.raf = requestAnimationFrame(this.boundFrame);
        return;
      }
    }
    this.lastFrame = now;

    r.setViewport(0, 0, RENDER_PX, RENDER_PX);
    for (const head of visible) {
      const t = this.reducedMotion ? 0 : now + head.phaseOffset;
      const { yaw, pitch } = head.animator.sample(t);
      head.scene!.update(yaw, pitch, t);
      this.poseHonk(head, now);
      r.render(head.scene!.scene, head.scene!.camera);
      head.ctx.clearRect(0, 0, RENDER_PX, RENDER_PX);
      head.ctx.drawImage(gl, 0, 0, RENDER_PX, RENDER_PX);
      const since = head.honking ? head.honkClock!.since(now) : null;
      if (since !== null)
        head.scene!.drawHonk(head.overlay!, since, RENDER_PX / 2);
    }

    // Reduced motion draws one frame per wake(); model changes re-wake.
    if (!this.reducedMotion) this.raf = requestAnimationFrame(this.boundFrame);
  }
}

let instance: SharedHeadRenderer | null = null;
export function getSharedHeadRenderer(): SharedHeadRenderer {
  if (!instance) instance = new SharedHeadRenderer();
  return instance;
}
