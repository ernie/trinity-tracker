import { useSyncExternalStore } from "react";

export interface MapMetaEntry {
  longname?: string;
  type?: string;
  fraglimit?: number;
  author?: string;
}

export interface MapMeta {
  shortName: string;
  longName?: string;
  /** longName when present, else shortName */
  displayName: string;
  type?: string;
}

// Module-scope cache. One fetch on first call; subscribers are notified on load.
let state: { loaded: boolean; data: Record<string, MapMetaEntry> } = {
  loaded: false,
  data: {},
};
const listeners = new Set<() => void>();
let fetchStarted = false;

function notify() {
  listeners.forEach((l) => l());
}

function startFetch() {
  if (fetchStarted) return;
  fetchStarted = true;
  fetch("/assets/maps.json")
    .then((res) => (res.ok ? res.json() : {}))
    .catch(() => ({}))
    .then((data: Record<string, MapMetaEntry>) => {
      state = { loaded: true, data: data || {} };
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
 * Returns map metadata for the given map id, synchronously. Until maps.json
 * loads, or if it's missing, displayName is the short name.
 */
export function useMapMeta(mapId: string | null | undefined): MapMeta {
  const snap = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const id = (mapId ?? "").toString();
  if (!id) return { shortName: "", displayName: "" };
  const entry = snap.data[id.toLowerCase()];
  return {
    shortName: id,
    longName: entry?.longname,
    displayName: entry?.longname ?? id,
    type: entry?.type,
  };
}
