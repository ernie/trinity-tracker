import { useState, useMemo, useEffect } from 'react'
import type { ActivityItem, Player, ServerStatus } from '../types'
import { BotBadge } from './BotBadge'
import { ColoredText } from './ColoredText'
import { PlayerBadge } from './PlayerBadge'
import { FlagIcon } from './FlagIcon'
import { MedalIcon } from './MedalIcon'
import { PlayerItem } from './PlayerItem'

interface ActivityLogProps {
  activities: ActivityItem[]
  servers: Map<number, ServerStatus>
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

// Strip Q3 color codes to get plain text for tooltips
function getPlainText(text: string): string {
  return text.replace(/\^[0-9]/g, '')
}

const STORAGE_KEY_SERVER_FILTER = 'q3a_activity_server_filter'
const STORAGE_KEY_INCLUDE_BOTS = 'q3a_activity_include_bots'

export function ActivityLog({ activities, servers, onPlayerClick }: ActivityLogProps) {
  const [serverFilter, setServerFilter] = useState<number | 'all'>(() => {
    const stored = localStorage.getItem(STORAGE_KEY_SERVER_FILTER)
    if (stored === null || stored === 'all') return 'all'
    const num = Number(stored)
    return isNaN(num) ? 'all' : num
  })
  const [includeBots, setIncludeBots] = useState(() => {
    return localStorage.getItem(STORAGE_KEY_INCLUDE_BOTS) === 'true'
  })

  // Persist filter state to localStorage
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY_SERVER_FILTER, String(serverFilter))
  }, [serverFilter])

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY_INCLUDE_BOTS, String(includeBots))
  }, [includeBots])

  // Get servers for filter dropdown - only show servers with real names
  const availableServers = useMemo(() => {
    return Array.from(servers.entries())
      .filter(([, status]) => status.name && !/^Server \d+$/.test(status.name))
      .map(([id, status]) => [id, status.name] as [number, string])
      .sort((a, b) => a[0] - b[0])
  }, [servers])

  // Aggregate human players across all online servers
  const humanPlayersWithServer = useMemo(() => {
    const players: Array<{ player: Player; serverName: string }> = []

    for (const [, status] of servers.entries()) {
      if (!status.players || !status.online) continue
      const serverName = status.name && !/^Server \d+$/.test(status.name) ? status.name : null
      if (!serverName) continue

      for (const player of status.players) {
        if (!player.is_bot && player.team !== 3) { // Exclude bots and spectators
          players.push({ player, serverName })
        }
      }
    }

    return players.sort((a, b) => b.player.score - a.player.score)
  }, [servers])

  // Filter activities
  const filteredActivities = useMemo(() => {
    return activities.filter((activity) => {
      // Server filter
      if (serverFilter !== 'all' && activity.serverId !== serverFilter) {
        return false
      }
      // Bot filter (always show match_start events)
      if (!includeBots && activity.player?.isBot && activity.activityType !== 'match_start') {
        return false
      }
      return true
    })
  }, [activities, serverFilter, includeBots])

  return (
    <div className="activity-log">
      {humanPlayersWithServer.length > 0 && (
        <div className="active-players-section">
          <div className="active-players-header">
            {humanPlayersWithServer.length} Playing
          </div>
          <ul className="active-players-list">
            {humanPlayersWithServer.map(({ player, serverName }) => (
              <div key={`${serverName}-${player.client_num}`} className="active-player-wrapper">
                <PlayerItem
                  player={player}
                  onClick={onPlayerClick ? () => onPlayerClick(player.name, player.clean_name, player.player_id) : undefined}
                />
                <span className="active-player-server">{serverName}</span>
              </div>
            ))}
          </ul>
        </div>
      )}
      <div className="activity-filters">
        <select
          value={serverFilter}
          onChange={(e) => setServerFilter(e.target.value === 'all' ? 'all' : Number(e.target.value))}
          className="server-filter"
        >
          <option value="all">All Servers</option>
          {availableServers.map(([id, name]) => (
            <option key={id} value={id}>{name}</option>
          ))}
        </select>
        <label className="include-bots-toggle">
          <input
            type="checkbox"
            checked={includeBots}
            onChange={(e) => setIncludeBots(e.target.checked)}
          />
          Include bots
        </label>
      </div>
      <div className="activity-list">
        {filteredActivities.map((activity) => (
          activity.activityType === 'match_start' && activity.mapName ? (
            <div
              key={activity.id}
              className="activity-item activity-map-change"
              style={{ backgroundImage: `url(/assets/levelshots/${activity.mapName.toLowerCase()}.jpg)` }}
            >
              <span className="activity-map-name">{activity.mapName}</span>
              {activity.serverName && (
                <span className="activity-map-server">{activity.serverName}</span>
              )}
            </div>
          ) : (
            <div key={activity.id} className="activity-item" title={getPlainText(activity.message)}>
              <span className="activity-time">
                {activity.timestamp.toLocaleTimeString()}
              </span>
              <span className={`activity-message ${activity.type}`}>
                <ActivityMessage
                  activity={activity}
                  onPlayerClick={onPlayerClick}
                />
              </span>
              {activity.serverName && (
                <span className="activity-server-watermark">{activity.serverName}</span>
              )}
            </div>
          )
        ))}
        {filteredActivities.length === 0 && (
          <div className="activity-item">
            <span className="activity-message info">Waiting for events...</span>
          </div>
        )}
      </div>
    </div>
  )
}

interface ActivityMessageProps {
  activity: ActivityItem
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

function ActivityMessage({ activity, onPlayerClick }: ActivityMessageProps) {
  const message = activity.message

  // If no player info or no click handler, just render the message
  if (!activity.player || !onPlayerClick) {
    return <ColoredText text={message} />
  }

  // Find the player name in the message and make it clickable
  const { name: playerName, cleanName, playerId, isBot, skill } = activity.player
  const playerNameWithReset = playerName + '^7'
  const idx = message.indexOf(playerNameWithReset)

  if (idx === -1) {
    // Fallback if we can't find the name
    return <ColoredText text={message} />
  }

  const before = message.slice(0, idx)
  let after = message.slice(idx + playerNameWithReset.length)

  // Determine which icon to show based on activity type
  const icon = getActivityIcon(activity)

  // Check if there's a victim (for humiliation awards)
  let victimElement: React.ReactNode = null
  let afterVictim = ''
  if (activity.victim) {
    const { name: victimName, cleanName: victimCleanName, playerId: victimPlayerId } = activity.victim
    const victimNameWithReset = victimName + '^7'
    const victimIdx = after.indexOf(victimNameWithReset)
    if (victimIdx !== -1) {
      const beforeVictim = after.slice(0, victimIdx)
      afterVictim = after.slice(victimIdx + victimNameWithReset.length)
      after = beforeVictim
      victimElement = (
        <span
          className="clickable-player"
          onClick={() => onPlayerClick(victimName, victimCleanName, victimPlayerId)}
        >
          <ColoredText text={victimName} />
        </span>
      )
    }
  }

  return (
    <>
      <ColoredText text={before} />
      {icon}
      {!icon && isBot && <BotBadge isBot skill={skill!} />}
      {!icon && !isBot && playerId && <PlayerBadge playerId={playerId} isVR={activity.player?.isVR} />}
      <span
        className="clickable-player"
        onClick={() => onPlayerClick(playerName, cleanName, playerId)}
      >
        <ColoredText text={playerName} />
      </span>
      <ColoredText text={'^7' + after} />
      {victimElement}
      {afterVictim && <ColoredText text={'^7' + afterVictim} />}
    </>
  )
}

function getActivityIcon(activity: ActivityItem): React.ReactNode {
  const { activityType, team, awardType } = activity

  // Flag events
  if (activityType === 'flag_capture') {
    return <MedalIcon type="capture" showCount={false} />
  }
  if (activityType === 'flag_taken' && team) {
    const flagTeam = team === 1 ? 'red' : 'blue'
    return <FlagIcon team={flagTeam} status="taken" size="sm" />
  }
  if (activityType === 'flag_return' && team) {
    const flagTeam = team === 1 ? 'red' : 'blue'
    return <FlagIcon team={flagTeam} status="base" size="sm" />
  }
  if (activityType === 'flag_drop' && team) {
    const flagTeam = team === 1 ? 'red' : 'blue'
    return <FlagIcon team={flagTeam} status="dropped" size="sm" />
  }

  // Award/medal events
  if (activityType === 'award' && awardType) {
    switch (awardType) {
      case 'impressive':
        return <MedalIcon type="impressive" showCount={false} />
      case 'excellent':
        return <MedalIcon type="excellent" showCount={false} />
      case 'humiliation':
        return <MedalIcon type="humiliation" showCount={false} />
      case 'defend':
        return <MedalIcon type="defend" showCount={false} />
      case 'assist':
        return <MedalIcon type="assist" showCount={false} />
    }
  }

  return null
}
