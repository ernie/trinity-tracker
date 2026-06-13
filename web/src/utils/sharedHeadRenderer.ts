import * as THREE from "three";
import { HeadScene } from "./headScene";
import { HeadIdleAnimator } from "./headIdleAngles";
import { frameIntervalMs, phaseOffsetFor } from "./headRenderMath";

export interface HeadHandle {
  release(): void;
}

// Square render resolution per head; each head's target canvas is sized to
// match and CSS-scaled to its box.
const RENDER_PX = 256;

interface HeadEntry {
  canvas: HTMLCanvasElement;
  ctx: CanvasRenderingContext2D;
  model: string;
  skin: string;
  scene: HeadScene | null;
  animator: HeadIdleAnimator;
  phaseOffset: number;
  visible: boolean;
  loaded: boolean;
  // Bumped on context loss so a load in flight at that moment can't install a
  // scene built against the dead context.
  loadGen: number;
  onLive: () => void;
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
      loaded: false,
      loadGen: 0,
      onLive,
    };
    this.entries.add(entry);
    this.io!.observe(canvas);
    void this.loadEntry(entry);
    return { release: () => this.release(entry) };
  }

  private async loadEntry(entry: HeadEntry): Promise<void> {
    const gen = entry.loadGen;
    const scene = new HeadScene();
    try {
      await scene.load(entry.model, entry.skin);
    } catch {
      scene.dispose();
      // A missing head bundle is expected, not an error — the poster stands in.
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
        if (head.canvas === e.target) head.visible = e.isIntersecting;
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
    }
  };

  private onContextRestored = () => {
    for (const head of this.entries) void this.loadEntry(head);
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
      r.render(head.scene!.scene, head.scene!.camera);
      head.ctx.clearRect(0, 0, RENDER_PX, RENDER_PX);
      head.ctx.drawImage(gl, 0, 0, RENDER_PX, RENDER_PX);
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
