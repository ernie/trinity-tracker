import { formatNumber } from "../utils";

interface HeadlineStatProps {
  label: string;
  /** Numbers get thousands separators; strings, such as a K/D ratio,
   *  show as given. */
  value: number | string;
  title?: string;
}

export function HeadlineStat({ label, value, title }: HeadlineStatProps) {
  const display = typeof value === "number" ? formatNumber(value) : value;
  return (
    <div className="headline-stat" title={title}>
      <div className="headline-stat__value">{display}</div>
      <div className="headline-stat__label">{label}</div>
    </div>
  );
}
