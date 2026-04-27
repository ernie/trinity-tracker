import { usePlayerSessions } from '../hooks/usePlayerSessions'
import { formatDateTime, formatDuration } from '../utils/formatters'

interface PlayerSessionsProps {
  playerId: number
  token: string
}

export function PlayerSessions({ playerId, token }: PlayerSessionsProps) {
  const { sessions, loading, error, hasMore, loadMore } = usePlayerSessions(
    playerId,
    token
  )

  if (error) {
    return (
      <div className="player-sessions-section">
        <h4>Recent Sessions</h4>
        <div className="sessions-error">{error}</div>
      </div>
    )
  }

  if (loading && sessions.length === 0) {
    return (
      <div className="player-sessions-section">
        <h4>Recent Sessions</h4>
        <div className="sessions-loading">Loading sessions...</div>
      </div>
    )
  }

  if (sessions.length === 0) {
    return (
      <div className="player-sessions-section">
        <h4>Recent Sessions</h4>
        <div className="sessions-empty">No sessions found</div>
      </div>
    )
  }

  return (
    <div className="player-sessions-section">
      <h4>Recent Sessions</h4>
      <table className="sessions-table">
        <thead>
          <tr>
            <th>Server</th>
            <th>Joined</th>
            <th>Duration</th>
            <th>Engine</th>
            <th>Trinity</th>
            <th>IP Address</th>
          </tr>
        </thead>
        <tbody>
          {sessions.map((session) => (
            <tr key={session.id}>
              <td>{session.server_source} / {session.server_key}</td>
              <td>{formatDateTime(session.joined_at)}</td>
              <td>
                {session.duration_seconds
                  ? formatDuration(session.duration_seconds)
                  : 'Active'}
              </td>
              <td className="client-engine">{session.client_engine || '-'}</td>
              <td className="client-version">{session.client_version || '-'}</td>
              <td className="ip-address">{session.ip_address || '-'}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {hasMore && (
        <button
          className="load-more-btn"
          onClick={loadMore}
          disabled={loading}
        >
          {loading ? 'Loading...' : 'Load More'}
        </button>
      )}
    </div>
  )
}
