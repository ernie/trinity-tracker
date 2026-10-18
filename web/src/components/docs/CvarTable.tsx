import { PlatformChip } from "./PlatformChip";
import { usePlatform } from "./PlatformContext";
import type { CvarEntry } from "../../data/cvars";

interface CvarTableProps {
  entries: CvarEntry[];
  hidePlatforms?: boolean;
}

export function CvarTable({ entries, hidePlatforms }: CvarTableProps) {
  const { platform } = usePlatform();
  if (entries.length === 0) {
    return <p className="docs-reference__empty">No matches.</p>;
  }
  return (
    <ul className="cvar-list">
      {entries.map((entry) => {
        const value = entry.defaultByPlatform?.[platform] ?? entry.default;
        return (
          <li key={entry.name} className="cvar-entry">
            <div className="cvar-entry__head">
              <code className="cvar-entry__name">{entry.name}</code>
              <span className="cvar-entry__default">
                default <code>{value === "" ? '""' : value}</code>
              </span>
              {!hidePlatforms && <PlatformChip platform={entry.platforms} />}
            </div>
            <p className="cvar-entry__description">{entry.description}</p>
            {entry.values && (
              <ul className="cvar-entry__values">
                {entry.values.map((v) => (
                  <li key={v.value}>
                    <code>{v.value}</code> {v.meaning}
                  </li>
                ))}
              </ul>
            )}
            {entry.notes && <p className="cvar-entry__notes">{entry.notes}</p>}
          </li>
        );
      })}
    </ul>
  );
}
