import { useEffect } from "react";
import { ActivityLog } from "./ActivityLog";
import { CloseIcon } from "./CloseIcon";
import type { ActivityItem, ServerStatus } from "../types";

interface ActivityDrawerProps {
  activities: ActivityItem[];
  servers: Map<number, ServerStatus>;
  onClose: () => void;
  onPlayerClick?: (
    playerName: string,
    cleanName: string,
    playerId?: number,
  ) => void;
}

// Slides in from the right (matches MyServersDrawer). Only mounted when
// open, so the mount itself is the open/close gesture — no explicit
// transition needed for first cut.
export function ActivityDrawer({
  activities,
  servers,
  onClose,
  onPlayerClick,
}: ActivityDrawerProps) {
  // Esc to close. Bound at document level so focus inside the drawer
  // still reaches the handler.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="drawer-overlay" onClick={onClose}>
      <aside
        className="drawer activity-drawer"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-label="Activity"
      >
        <div className="drawer-header">
          <h2>Activity</h2>
          <button className="close-btn" onClick={onClose} aria-label="Close">
            <CloseIcon />
          </button>
        </div>
        <div className="drawer-body activity-drawer__body">
          <ActivityLog
            activities={activities}
            servers={servers}
            onPlayerClick={onPlayerClick}
          />
        </div>
      </aside>
    </div>
  );
}
