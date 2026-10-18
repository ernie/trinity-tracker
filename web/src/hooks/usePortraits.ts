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
      // A stale object-shaped manifest gives an empty picker.
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
 * Portrait icons for the picker. A missing portraits.json loads as an empty
 * list.
 */
export function usePortraitManifest(): {
  loaded: boolean;
  data: PortraitManifest;
} {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
