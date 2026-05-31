import type { ReactNode } from "react";
import { formatNumber } from "../utils";

export interface StatItemProps {
  label: string;
  value: number | string;
  className?: string;
  subscript?: number;
  title?: string;
  /** Background "whisper" icon at the lower-right of the cell. Pass a
   *  URL string for the common single-image case, or any ReactNode
   *  (e.g. a <FlagPair />) for composed emblems. Falls back to a faint
   *  "#" glyph when omitted. The wrapper carries the rotate/opacity
   *  treatment, so the inner content inherits it automatically. */
  backgroundIcon?: string | ReactNode;
}

export function StatItem({
  label,
  value,
  className,
  subscript,
  title,
  backgroundIcon,
}: StatItemProps) {
  const displayValue = typeof value === "number" ? formatNumber(value) : value;
  const displaySubscript =
    subscript !== undefined && subscript > 0 ? formatNumber(subscript) : null;

  return (
    <div className="stat-item" title={title}>
      {typeof backgroundIcon === "string" ? (
        <img className="stat-item-bg-icon" src={backgroundIcon} alt="" />
      ) : backgroundIcon ? (
        <span className="stat-item-bg-icon">{backgroundIcon}</span>
      ) : (
        <span className="stat-item-bg-icon stat-item-bg-hash">#</span>
      )}
      <div className={`stat-value ${className ?? ""}`}>
        {displayValue}
        {displaySubscript && <sub>{displaySubscript}</sub>}
      </div>
      <div className="stat-label">{label}</div>
    </div>
  );
}
