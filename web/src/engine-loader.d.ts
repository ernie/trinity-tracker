// Types for the WASM engine loader, web/public/engine/loader.js from
// trinity-engine, which is imported at runtime from an absolute URL.
import type { EngineModule } from "./types";

export interface LoadEngineOptions {
  canvas: HTMLCanvasElement;
  statusEl: HTMLElement;
  enginePath: string;
  configUrl?: string;
  /** VOD: URL of a finished .tvd. Mutually exclusive with liveUrl. */
  demoUrl?: string;
  /** Live: URL of a growing TVL1 stream. Mutually exclusive with demoUrl. */
  liveUrl?: string;
  /** Live only: explicit game dir (TVL1 header carries no configstrings). */
  fsGame?: string;
  extraPk3s?: string[];
  extraArgs?: string;
  authToken?: string;
  onProgress?: (loaded: number, total: number) => void;
  onReady?: () => void;
}

export declare function loadEngine(
  opts: LoadEngineOptions,
): Promise<EngineModule>;
