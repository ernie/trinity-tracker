import { useSyncExternalStore } from "react";

/** Ordered picker entries (baseq3 models first, then Team Arena), served
 *  as /assets/portraits.json by `trinity portraits`. */
export type PortraitManifest = { model: string; skins: string[] }[];

let state: { loaded: boolean; data: PortraitManifest } = {
  loaded: false,
  data: [],
};
const listeners = new Set<() => void>();
let fetchStarted = false;

function notify() {
  listeners.forEach((l) => l());
}

function startFetch() {
  if (fetchStarted) return;
  fetchStarted = true;
  fetch("/assets/portraits.json")
    .then((res) => (res.ok ? res.json() : []))
    .catch(() => [])
    .then((data: unknown) => {
      // A stale object-shaped manifest yields an empty picker, not a crash.
      state = {
        loaded: true,
        data: Array.isArray(data) ? (data as PortraitManifest) : [],
      };
      notify();
    });
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  startFetch();
  return () => listeners.delete(cb);
}

function getSnapshot() {
  return state;
}

/**
 * Available portrait icons. `data` stays empty until the manifest loads;
 * a missing portraits.json is non-fatal and resolves to loaded=true with
 * no entries.
 */
export function usePortraitManifest(): {
  loaded: boolean;
  data: PortraitManifest;
} {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
