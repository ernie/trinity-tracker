import { useSources } from "../hooks/useSources";

interface SourceFilterProps {
  value: string; // empty string means "all sources"
  onChange: (next: string) => void;
  className?: string;
}

// Inactive sources stay listed so older matches can still be filtered by them.
export function SourceFilter({
  value,
  onChange,
  className,
}: SourceFilterProps) {
  const { sources, hasMultiple } = useSources();
  if (!hasMultiple) return null;

  return (
    <select
      className={`source-filter${className ? " " + className : ""}`}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      aria-label="Filter by source"
    >
      <option value="">All sources</option>
      {sources.map((s) => (
        <option key={s.source} value={s.source}>
          {s.source}
          {s.active ? "" : " (inactive)"}
        </option>
      ))}
    </select>
  );
}
