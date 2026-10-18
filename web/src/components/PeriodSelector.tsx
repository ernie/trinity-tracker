import { PERIOD_LABELS } from "../constants/labels";
import { NavScroller } from "./NavScroller";
import type { TimePeriod } from "../types";

interface PeriodSelectorProps {
  period: TimePeriod;
  onChange: (period: TimePeriod) => void;
}

export function PeriodSelector({ period, onChange }: PeriodSelectorProps) {
  return (
    <div className="period-selector">
      <NavScroller scrollClassName="filter-chips__scroll">
        <div className="filter-chips__strip">
          {(Object.keys(PERIOD_LABELS) as TimePeriod[]).map((p) => (
            <button
              key={p}
              className={`period-btn ${period === p ? "active" : ""}`}
              onClick={() => onChange(p)}
            >
              {PERIOD_LABELS[p]}
            </button>
          ))}
        </div>
      </NavScroller>
    </div>
  );
}
