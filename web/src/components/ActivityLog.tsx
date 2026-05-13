import { useState, useMemo, useEffect } from 'react'
import type { ActivityItem, Player, ServerStatus } from '../types'
import { BotBadge } from './BotBadge'
import { ColoredText } from './ColoredText'
import { PlayerBadge } from './PlayerBadge'
import { FlagIcon } from './FlagIcon'
import { MedalIcon } from './MedalIcon'
import { SkullIcon } from './SkullIcon'
import { TargetReticleIcon } from './TargetReticleIcon'
import { PlayerItem } from './PlayerItem'
import {
  ActivityFilters,
  loadActivityFilters,
  type ActivityFilterState,
} from './ActivityFilters'
import { classifyActivity } from '../constants/activityTaxonomy'
import { useSources } from '../hooks/useSources'
import { serverDisplay } from '../utils'

interface ActivityLogProps {
  activities: ActivityItem[]
  servers: Map<number, ServerStatus>
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

function getPlainText(text: string): string {
  return text.replace(/\^[0-9]/g, '')
}

export function ActivityLog({ activities, servers, onPlayerClick }: ActivityLogProps) {
  const { hasMultiple: hasMultipleSources } = useSources()
  const [filters, setFilters] = useState<ActivityFilterState>(() => loadActivityFilters())

  // Drop legacy per-axis filter keys once on mount so the old prefs
  // don't linger. Prior versions persisted three separate keys
  // (server / source / include-bots); the new model collapses them
  // into q3a_activity_filters.
  useEffect(() => {
    localStorage.removeItem('q3a_activity_server_filter')
    localStorage.removeItem('q3a_activity_source_filter')
    localStorage.removeItem('q3a_activity_include_bots')
  }, [])

  // Live human players grouped by server, for the "X playing" header.
  const humanPlayersWithServer = useMemo(() => {
    const players: Array<{ player: Player; serverName: string }> = []
    for (const [, status] of servers.entries()) {
      if (!status.players || !status.online || !status.key) continue
      const serverName = serverDisplay(status.source, status.key, { hasMultipleSources })
      for (const player of status.players) {
        if (!player.is_bot && player.team !== 3) {
          players.push({ player, serverName })
        }
      }
    }
    return players.sort((a, b) => b.player.score - a.player.score)
  }, [servers, hasMultipleSources])

  // Apply all filter axes in series. Bot filtering is orthogonal to the
  // categorical filters (it's a player-attribute check). Match-start
  // events have no player attribution and are never bot-filtered.
  const filteredActivities = useMemo(() => {
    const hiddenSources = new Set(filters.hiddenSources)
    const hiddenServers = new Set(filters.hiddenServers)
    const hiddenEvents = new Set(filters.hiddenEvents)
    const hiddenGameTypes = new Set(filters.hiddenGameTypes)
    return activities.filter((a) => {
      const status = a.serverId !== undefined ? servers.get(a.serverId) : undefined
      if (status?.source && hiddenSources.has(status.source)) return false
      if (a.serverId !== undefined && hiddenServers.has(a.serverId)) return false
      if (status?.game_type && hiddenGameTypes.has(status.game_type.toLowerCase())) return false
      const key = classifyActivity(a)
      if (key && hiddenEvents.has(key)) return false
      if (!filters.includeBots && a.player?.isBot && a.activityType !== 'match_start') return false
      return true
    })
  }, [activities, filters, servers])

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

      <details className="activity-filters-toggle">
        <summary>Filters</summary>
        <ActivityFilters state={filters} onChange={setFilters} servers={servers} />
      </details>

      <div className="activity-list">
        {filteredActivities.map((activity) => (
          activity.activityType === 'match_start' && activity.mapName ? (
            <div
              key={activity.id}
              className="activity-item activity-map-change"
              style={{ backgroundImage: `url(/assets/levelshots/${activity.mapName.toLowerCase()}.jpg)` }}
            >
              <div className="activity-map-text">
                <span className="activity-map-name">{activity.mapName}</span>
                <span className="activity-map-time">{activity.timestamp.toLocaleTimeString()}</span>
              </div>
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
            <span className="activity-message info">
              {activities.length === 0 ? 'Waiting for events...' : 'No events match the current filters'}
            </span>
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

  if (!activity.player || !onPlayerClick) {
    return <ColoredText text={message} />
  }

  const { name: playerName, cleanName, playerId, isBot, skill } = activity.player
  const playerNameWithReset = playerName + '^7'
  const idx = message.indexOf(playerNameWithReset)

  if (idx === -1) {
    return <ColoredText text={message} />
  }

  const before = message.slice(0, idx)
  let after = message.slice(idx + playerNameWithReset.length)

  const icon = getActivityIcon(activity)

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
      {!icon && !isBot && playerId && <PlayerBadge isVerified={activity.player?.isVerified} isAdmin={activity.player?.isAdmin} isVR={activity.player?.isVR} />}
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

  if (activityType === 'flag_capture') {
    return <MedalIcon type="capture" showCount={false} />
  }
  if (activityType === 'flag_taken' && team !== undefined) {
    const flagTeam = team === 1 ? 'red' : team === 2 ? 'blue' : 'neutral'
    return <FlagIcon team={flagTeam} status="taken" size="sm" />
  }
  if (activityType === 'flag_return' && team !== undefined) {
    const flagTeam = team === 1 ? 'red' : team === 2 ? 'blue' : 'neutral'
    return <FlagIcon team={flagTeam} status="base" size="sm" />
  }
  if (activityType === 'flag_drop' && team !== undefined) {
    const flagTeam = team === 1 ? 'red' : team === 2 ? 'blue' : 'neutral'
    return <FlagIcon team={flagTeam} status="dropped" size="sm" />
  }

  if (activityType === 'award' && awardType) {
    switch (awardType) {
      case 'impressive': return <MedalIcon type="impressive" showCount={false} />
      case 'excellent': return <MedalIcon type="excellent" showCount={false} />
      case 'humiliation': return <MedalIcon type="humiliation" showCount={false} />
      case 'defend': return <MedalIcon type="defend" showCount={false} />
      case 'assist': return <MedalIcon type="assist" showCount={false} />
    }
  }

  // Transient pickup uses the carrier silhouette; medal disc is reserved
  // for the cumulative score event. Skull color is the carrier's enemy.
  if (activityType === 'skull_pickup' && team !== undefined) {
    const skullTeam = team === 1 ? 'blue' : 'red'
    return <SkullIcon team={skullTeam} title="Skull picked up" />
  }
  if (activityType === 'skull_score') {
    return <MedalIcon type="skull" showCount={false} />
  }
  // Transient damage gets the attacker glyph; destroy keeps the medal disc.
  // event.team is the obelisk being hit, which is also the reticle color.
  if (activityType === 'obelisk_damage' && team !== undefined) {
    const obeliskTeam = team === 1 ? 'red' : 'blue'
    return <TargetReticleIcon team={obeliskTeam} size="sm" title="Attacking obelisk" />
  }
  if (activityType === 'obelisk_destroy') {
    return <MedalIcon type="obelisk" showCount={false} />
  }

  return null
}
