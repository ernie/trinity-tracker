import { useEffect, useState } from "react";

export interface SourceInfo {
  source: string;
  active: boolean;
}

let cached: SourceInfo[] | null = null;
let cachedAt = 0;
const CACHE_MS = 60_000;

// useSources fetches /api/sources and caches the result for a minute.
export function useSources(): { sources: SourceInfo[]; hasMultiple: boolean } {
  const [sources, setSources] = useState<SourceInfo[]>(() =>
    cached && Date.now() - cachedAt < CACHE_MS ? cached : [],
  );

  useEffect(() => {
    // A fresh cache is already in the initial state.
    if (cached && Date.now() - cachedAt < CACHE_MS) return;
    let cancelled = false;
    fetch("/api/sources")
      .then((r) => (r.ok ? r.json() : []))
      .then((list: SourceInfo[]) => {
        if (cancelled) return;
        cached = list || [];
        cachedAt = Date.now();
        setSources(cached);
      })
      .catch(() => {
        // An empty list just shows no source tags.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { sources, hasMultiple: sources.length > 1 };
}
