import { PlatformChip } from "./PlatformChip";
import type { CvarEntry } from "../../data/cvars";

interface CvarTableProps {
  entries: CvarEntry[];
  // When true, suppress the per-cvar platform chips (used by the Server
  // CVars section, where every entry applies to any Trinity server
  // regardless of which client architecture connects).
  hidePlatforms?: boolean;
}

// Renders a list of cvars as cards: name + default + platform chips
// in the head row, description below, optional value enumeration and
// notes. Receives an already-filtered list — filtering happens in the
// parent.
export function CvarTable({ entries, hidePlatforms }: CvarTableProps) {
  if (entries.length === 0) {
    return <p className="docs-reference__empty">No matches.</p>;
  }
  return (
    <ul className="cvar-list">
      {entries.map((entry) => (
        <li key={entry.name} className="cvar-entry">
          <div className="cvar-entry__head">
            <code className="cvar-entry__name">{entry.name}</code>
            <span className="cvar-entry__default">
              default <code>{entry.default === "" ? '""' : entry.default}</code>
            </span>
            {!hidePlatforms && <PlatformChip platform={entry.platforms} />}
          </div>
          <p className="cvar-entry__description">{entry.description}</p>
          {entry.values && (
            <ul className="cvar-entry__values">
              {entry.values.map((v) => (
                <li key={v.value}>
                  <code>{v.value}</code> — {v.meaning}
                </li>
              ))}
            </ul>
          )}
          {entry.notes && <p className="cvar-entry__notes">{entry.notes}</p>}
        </li>
      ))}
    </ul>
  );
}
