import type { ReactNode } from "react";
import { formatNumber } from "../utils";

export interface StatItemProps {
  label: string;
  value: number | string;
  className?: string;
  subscript?: number;
  title?: string;
  /** Faint icon at the cell's lower right: an image URL, or any node
   *  such as a <FlagPair />. Defaults to a "#" glyph. The wrapper applies
   *  the rotation and opacity. */
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
