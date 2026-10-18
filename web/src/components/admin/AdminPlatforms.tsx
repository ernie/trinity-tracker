import { useEffect, useState } from "react";
import { apiFetch } from "../../authFetch";
import { formatDuration } from "../../utils/formatters";
import { platformName } from "../../utils/platform";
import type { PlatformStats } from "../../types";

const WINDOWS = [
  { key: "7d", label: "Last 7 days", days: 7 },
  { key: "30d", label: "Last 30 days", days: 30 },
  { key: "90d", label: "Last 90 days", days: 90 },
  { key: "all", label: "All time", days: 0 },
] as const;

type WindowKey = (typeof WINDOWS)[number]["key"];

function percent(part: number, whole: number): string {
  return whole > 0 ? `${Math.round((part / whole) * 100)}%` : "—";
}

export function AdminPlatforms() {
  const [windowKey, setWindowKey] = useState<WindowKey>("30d");
  const [stats, setStats] = useState<PlatformStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const days = WINDOWS.find((w) => w.key === windowKey)?.days ?? 0;
    const params = new URLSearchParams();
    if (days > 0) {
      params.set(
        "since",
        new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString(),
      );
    }
    let cancelled = false;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    setError("");
    apiFetch(`/api/admin/platform-stats?${params.toString()}`)
      .then(async (res) => {
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          throw new Error(data.error || "Couldn't load platform stats.");
        }
        return res.json();
      })
      .then((data: PlatformStats) => {
        if (!cancelled) setStats(data);
      })
      .catch((err) => {
        if (!cancelled) {
          setError(
            err instanceof Error
              ? err.message
              : "Couldn't load platform stats.",
          );
          setStats(null);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [windowKey]);

  return (
    <div className="admin-platforms">
      <div className="admin-section-header">
        <h2>Platforms</h2>
      </div>

      <div className="admin-preset-chips">
        {WINDOWS.map((w) => (
          <button
            key={w.key}
            type="button"
            className={`admin-preset-chip ${windowKey === w.key ? "active" : ""}`}
            onClick={() => setWindowKey(w.key)}
          >
            {w.label}
          </button>
        ))}
      </div>

      {error && <div className="error-message">{error}</div>}

      {stats && (
        <>
          <dl className="platform-totals">
            <div>
              <dt>Players</dt>
              <dd>{stats.players}</dd>
            </div>
            <div>
              <dt>Played in VR</dt>
              <dd>
                {stats.vr_players} ({percent(stats.vr_players, stats.players)})
              </dd>
            </div>
            <div>
              <dt>Sessions</dt>
              <dd>{stats.sessions}</dd>
            </div>
            <div>
              <dt>Play time</dt>
              <dd>{formatDuration(stats.playtime_seconds)}</dd>
            </div>
          </dl>

          <table className="admin-data-table platform-stats-table">
            <thead>
              <tr>
                <th>Platform</th>
                <th>Mode</th>
                <th>Players</th>
                <th>Sessions</th>
                <th>Play time</th>
                <th>Share of play time</th>
              </tr>
            </thead>
            <tbody>
              {stats.rows.map((r) => (
                <tr key={`${r.platform}-${r.is_vr}`}>
                  <td data-label="Platform">{platformName(r.platform)}</td>
                  <td data-label="Mode">
                    <span className="session-platform">
                      {r.is_vr ? "VR" : "Flatscreen"}
                      {r.is_vr && (
                        <img
                          className="session-platform__vr"
                          src="/assets/vr/vr.png"
                          alt=""
                        />
                      )}
                    </span>
                  </td>
                  <td data-label="Players">{r.players}</td>
                  <td data-label="Sessions">{r.sessions}</td>
                  <td data-label="Play time">
                    {formatDuration(r.playtime_seconds)}
                  </td>
                  <td data-label="Share of play time">
                    {percent(r.playtime_seconds, stats.playtime_seconds)}
                  </td>
                </tr>
              ))}
              {stats.rows.length === 0 && !loading && (
                <tr>
                  <td colSpan={6} className="admin-empty">
                    No sessions in this window.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
          <p className="platform-stats-note">
            A session counts as VR once the player has run VR in it. Play time
            covers finished sessions. Sessions from before the tracker recorded
            platforms show as Not recorded.
          </p>
        </>
      )}

      {loading && <div className="admin-pagination">Loading…</div>}
    </div>
  );
}
