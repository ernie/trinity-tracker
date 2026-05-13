import {
  createContext, useCallback, useContext, useEffect, useMemo, useRef, useState,
  type ReactNode,
} from 'react'
import { useWebSocket } from '../useWebSocket'
import { useAuth } from '../hooks/useAuth'
import { useSources } from '../hooks/useSources'
import { serverDisplay } from '../utils'
import type {
  Server, ServerStatus, ActivityItem, ActivityPlayer, ActivityType, WSEvent,
  PlayerJoinData, PlayerLeaveData, MatchStartData,
  FlagCaptureData, FlagTakenData, FlagReturnData, FlagDropData,
  ObeliskDestroyData, ObeliskDamageData, SkullScoreData, SkullPickupData, TeamChangeData,
  SayData, SayTeamData, TellData, SayRconData, AwardData,
} from '../types'

// Backstop only — the collector emits explicit stop events; this
// catches the rare case where one is lost (e.g. collector restart).
const OBELISK_ATTACK_DECAY_MS = 30_000
const OBELISK_ATTACK_PRUNE_INTERVAL_MS = 1000

const COLOR_RESET = '^7'
const cleanQ3Name = (name: string) => name.replace(/\^[0-9]/g, '')

interface SelectedPlayer {
  name: string
  playerId: number
}

interface LiveDataValue {
  // Live state
  servers: Map<number, ServerStatus>
  liveness: Map<number, 'live' | 'stale' | 'offline'>
  manageable: Map<number, boolean>
  activities: ActivityItem[]
  newPlayers: Set<string>
  loading: boolean
  isConnected: boolean
  // serverId → client_nums currently attacking that server's obelisk.
  obeliskAttackersByServer: Map<number, Set<number>>

  // Derived counts (fed into the StatusPill).
  activeHumanPlayersCount: number
  activeServersCount: number

  // Activity drawer (universal — accessible from every route via the StatusPill).
  activityDrawerOpen: boolean
  setActivityDrawerOpen: (open: boolean) => void
  toggleActivityDrawer: () => void

  // Player-stats modal — universal because every page has clickable players.
  selectedPlayer: SelectedPlayer | null
  showPlayer: (playerName: string, cleanName: string, playerId?: number) => void
  closePlayer: () => void
  /** Monotonic counter — bumps every time the user clicks "View full
   *  profile" inside the player-stats modal. Pages that host search /
   *  list state (e.g. PlayersPage) can watch the counter to detect the
   *  drill-in commit even when the URL doesn't change (clicking through
   *  to the player you're already viewing). */
  drillInVersion: number
  notifyDrillIn: () => void

  // Forced password-change modal — needs to surface regardless of route.
  showPasswordChange: boolean
  setShowPasswordChange: (open: boolean) => void

  // Command palette (⌘K / Ctrl+K).
  commandPaletteOpen: boolean
  setCommandPaletteOpen: (open: boolean) => void
}

const LiveDataContext = createContext<LiveDataValue | null>(null)

export function useLiveData(): LiveDataValue {
  const ctx = useContext(LiveDataContext)
  if (!ctx) throw new Error('useLiveData must be used inside <LiveDataProvider>')
  return ctx
}

export function LiveDataProvider({ children }: { children: ReactNode }) {
  const { auth } = useAuth()
  const { hasMultiple: hasMultipleSources } = useSources()

  const [servers, setServers] = useState<Map<number, ServerStatus>>(new Map())
  const [liveness, setLiveness] = useState<Map<number, 'live' | 'stale' | 'offline'>>(new Map())
  const [manageable, setManageable] = useState<Map<number, boolean>>(new Map())
  const [activities, setActivities] = useState<ActivityItem[]>([])
  const [newPlayers, setNewPlayers] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [activityDrawerOpen, setActivityDrawerOpen] = useState(false)
  const [selectedPlayer, setSelectedPlayer] = useState<SelectedPlayer | null>(null)
  const [showPasswordChange, setShowPasswordChange] = useState(false)
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false)

  // Ref holds per-slot expiry timestamps; state mirrors live membership.
  const obeliskAttackersExpiryRef = useRef<Map<number, Map<number, number>>>(new Map())
  const [obeliskAttackersByServer, setObeliskAttackersByServer] = useState<Map<number, Set<number>>>(new Map())

  // visibleServerIdsRef gates WebSocket server_update events: RemotePoller
  // keeps polling every active+handshake server (it doesn't know about
  // liveness), so a hidden server's update would otherwise re-add it to
  // the map immediately after the prune. null = "list not fetched yet";
  // allow updates through until the first /api/servers response lands.
  const visibleServerIdsRef = useRef<Set<number> | null>(null)
  const activityIdRef = useRef(0)

  // Mirror of `servers` for callbacks that fire from event handlers.
  const serversRef = useRef<Map<number, ServerStatus>>(new Map())
  useEffect(() => { serversRef.current = servers }, [servers])

  // Aggregated counts for the StatusPill.
  const { activeHumanPlayersCount, activeServersCount } = useMemo(() => {
    let humans = 0
    let serversWithHumans = 0
    for (const [, status] of servers.entries()) {
      if (!status.players || !status.online || !status.key) continue
      let serverHasHuman = false
      for (const player of status.players) {
        if (!player.is_bot && player.team !== 3) {
          humans++
          serverHasHuman = true
        }
      }
      if (serverHasHuman) serversWithHumans++
    }
    return { activeHumanPlayersCount: humans, activeServersCount: serversWithHumans }
  }, [servers])

  const getServerName = useCallback((serverId: number): string | undefined => {
    const server = serversRef.current.get(serverId)
    if (!server?.key) return undefined
    return serverDisplay(server.source, server.key, { hasMultipleSources })
  }, [hasMultipleSources])

  const getPlayerBotInfo = useCallback(
    (serverId: number, cleanName: string) => {
      const server = serversRef.current.get(serverId)
      if (!server?.players) return { isBot: false }
      const p = server.players.find((x) => x.clean_name === cleanName)
      return {
        isBot: p?.is_bot ?? false,
        skill: p?.skill,
        isVR: p?.is_vr,
        isVerified: p?.is_verified,
        isAdmin: p?.is_admin,
      }
    },
    [],
  )

  const getServerGameType = useCallback((serverId: number): string | undefined => {
    return serversRef.current.get(serverId)?.game_type
  }, [])

  // Prune expired entries and republish the slot snapshot. Only
  // commits a new Map when membership actually changes.
  const refreshObeliskAttackerState = useCallback(() => {
    const now = Date.now()
    setObeliskAttackersByServer((prev) => {
      const next = new Map<number, Set<number>>()
      let changed = false
      for (const [serverId, expiryMap] of obeliskAttackersExpiryRef.current.entries()) {
        const live = new Set<number>()
        for (const [clientNum, expiresAt] of expiryMap.entries()) {
          if (expiresAt > now) live.add(clientNum)
          else expiryMap.delete(clientNum)
        }
        if (expiryMap.size === 0) {
          obeliskAttackersExpiryRef.current.delete(serverId)
        }
        if (live.size === 0) continue
        next.set(serverId, live)
        const prevSet = prev.get(serverId)
        if (!prevSet || prevSet.size !== live.size) { changed = true; continue }
        for (const cn of live) if (!prevSet.has(cn)) { changed = true; break }
      }
      // A server falling off the map (last attacker decayed) is also a change.
      if (!changed) for (const serverId of prev.keys()) if (!next.has(serverId)) { changed = true; break }
      return changed ? next : prev
    })
  }, [])

  useEffect(() => {
    const id = setInterval(refreshObeliskAttackerState, OBELISK_ATTACK_PRUNE_INTERVAL_MS)
    return () => clearInterval(id)
  }, [refreshObeliskAttackerState])

  const addActivity = useCallback(
    (
      type: ActivityItem['type'],
      message: string,
      options?: {
        player?: ActivityPlayer
        serverId?: number
        serverName?: string
        activityType?: ActivityType
        team?: number
        awardType?: ActivityItem['awardType']
        mapName?: string
        victim?: ActivityPlayer
      },
    ) => {
      setActivities((prev) => {
        const next: ActivityItem = {
          id: ++activityIdRef.current,
          timestamp: new Date(),
          type,
          message,
          player: options?.player,
          serverId: options?.serverId,
          serverName: options?.serverName,
          activityType: options?.activityType,
          team: options?.team,
          awardType: options?.awardType,
          mapName: options?.mapName,
          victim: options?.victim,
        }
        return [next, ...prev].slice(0, 50)
      })
    },
    [],
  )

  // WebSocket event dispatch.
  const handleEvent = useCallback((event: WSEvent) => {
    switch (event.event) {
      case 'server_update': {
        if (
          visibleServerIdsRef.current !== null &&
          !visibleServerIdsRef.current.has(event.server_id)
        ) break
        const status = event.data as ServerStatus
        setServers((prev) => new Map(prev).set(event.server_id, status))
        break
      }
      case 'player_join': {
        const data = event.data as PlayerJoinData
        if (data.player.is_bot) break
        const serverName = getServerName(event.server_id)
        const player = {
          name: data.player.name,
          cleanName: data.player.clean_name,
          playerId: data.player_id,
          isBot: data.player.is_bot,
          skill: data.player.skill,
          isVerified: data.player.is_verified,
          isAdmin: data.player.is_admin,
        }
        addActivity('join', `${data.player.name}${COLOR_RESET} joined`, { player, serverId: event.server_id, serverName })
        if (!data.player.is_bot) {
          setNewPlayers((prev) => new Set(prev).add(data.player.clean_name))
          setTimeout(() => {
            setNewPlayers((prev) => {
              const n = new Set(prev); n.delete(data.player.clean_name); return n
            })
          }, 60000)
        }
        break
      }
      case 'player_leave': {
        const data = event.data as PlayerLeaveData
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        if (botInfo.isBot) break
        const serverName = getServerName(event.server_id)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        addActivity('leave', `${data.player_name}${COLOR_RESET} left`, { player, serverId: event.server_id, serverName })
        break
      }
      case 'match_start': {
        const data = event.data as MatchStartData
        const serverName = getServerName(event.server_id)
        addActivity('info', `New map: ${data.map}`, {
          serverId: event.server_id, serverName, activityType: 'match_start', mapName: data.map,
        })
        break
      }
      case 'flag_capture': {
        const data = event.data as FlagCaptureData
        const serverName = getServerName(event.server_id)
        const gameType = getServerGameType(event.server_id)
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        const message = gameType === '1fctf'
          ? `${data.player_name}${COLOR_RESET} captured the flag!`
          : `${data.player_name}${COLOR_RESET} captured the ${data.team === 1 ? 'Blue' : 'Red'} flag!`
        addActivity('info', message, { player, serverId: event.server_id, serverName, activityType: 'flag_capture', team: data.team })
        break
      }
      case 'flag_taken': {
        const data = event.data as FlagTakenData
        const serverName = getServerName(event.server_id)
        const gameType = getServerGameType(event.server_id)
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        const message = gameType === '1fctf'
          ? `${data.player_name}${COLOR_RESET} took the flag`
          : `${data.player_name}${COLOR_RESET} took the ${data.team === 1 ? 'Red' : 'Blue'} flag`
        addActivity('info', message, { player, serverId: event.server_id, serverName, activityType: 'flag_taken', team: data.team })
        break
      }
      case 'flag_return': {
        const data = event.data as FlagReturnData
        const serverName = getServerName(event.server_id)
        const gameType = getServerGameType(event.server_id)
        const isOneFlag = gameType === '1fctf'
        const flagTeam = data.team === 1 ? 'Red' : 'Blue'
        if (data.player_name) {
          const cleanName = cleanQ3Name(data.player_name)
          const botInfo = getPlayerBotInfo(event.server_id, cleanName)
          const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
          const message = isOneFlag
            ? `${data.player_name}${COLOR_RESET} returned the flag`
            : `${data.player_name}${COLOR_RESET} returned the ${flagTeam} flag`
          addActivity('info', message, { player, serverId: event.server_id, serverName, activityType: 'flag_return', team: data.team })
        } else {
          const message = isOneFlag ? `The flag was returned` : `The ${flagTeam} flag was returned`
          addActivity('info', message, { serverId: event.server_id, serverName, activityType: 'flag_return', team: data.team })
        }
        break
      }
      case 'flag_drop': {
        const data = event.data as FlagDropData
        const serverName = getServerName(event.server_id)
        const gameType = getServerGameType(event.server_id)
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        const message = gameType === '1fctf'
          ? `${data.player_name}${COLOR_RESET} dropped the flag`
          : `${data.player_name}${COLOR_RESET} dropped the ${data.team === 1 ? 'Red' : 'Blue'} flag`
        addActivity('info', message, { player, serverId: event.server_id, serverName, activityType: 'flag_drop', team: data.team })
        break
      }
      case 'obelisk_destroy': {
        const data = event.data as ObeliskDestroyData
        const serverName = getServerName(event.server_id)
        const teamName = data.team === 1 ? 'Red' : 'Blue'
        const cleanName = cleanQ3Name(data.attacker_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.attacker_name, cleanName, playerId: data.player_id, ...botInfo }
        addActivity('info', `${data.attacker_name}${COLOR_RESET} destroyed the ${teamName} obelisk!`, {
          player, serverId: event.server_id, serverName, activityType: 'obelisk_destroy', team: data.team,
        })
        // Clear any lingering attacker indicator; destroy is terminal.
        // Best-effort lookup by clean_name (destroy event has no client_num).
        if (data.player_id !== undefined) {
          const expiryMap = obeliskAttackersExpiryRef.current.get(event.server_id)
          if (expiryMap) {
            const status = serversRef.current.get(event.server_id)
            const slot = status?.players?.find((p) => p.clean_name === cleanName)?.client_num
            if (slot !== undefined && expiryMap.delete(slot)) refreshObeliskAttackerState()
          }
        }
        break
      }
      case 'obelisk_damage': {
        const data = event.data as ObeliskDamageData
        // Log only the start; stop events would just be noise.
        if (data.active) {
          const serverName = getServerName(event.server_id)
          const teamName = data.team === 1 ? 'Red' : 'Blue'
          const cleanName = cleanQ3Name(data.attacker_name)
          const botInfo = getPlayerBotInfo(event.server_id, cleanName)
          const player = { name: data.attacker_name, cleanName, playerId: data.player_id, ...botInfo }
          addActivity('info', `${data.attacker_name}${COLOR_RESET} is attacking the ${teamName} obelisk`, {
            player, serverId: event.server_id, serverName, activityType: 'obelisk_damage', team: data.team,
          })
        }
        // Expiry is the backstop; the matching stop event is the path.
        if (data.client_num >= 0) {
          let expiryMap = obeliskAttackersExpiryRef.current.get(event.server_id)
          if (data.active) {
            if (!expiryMap) {
              expiryMap = new Map()
              obeliskAttackersExpiryRef.current.set(event.server_id, expiryMap)
            }
            expiryMap.set(data.client_num, Date.now() + OBELISK_ATTACK_DECAY_MS)
          } else if (expiryMap) {
            expiryMap.delete(data.client_num)
          }
          refreshObeliskAttackerState()
        }
        break
      }
      case 'skull_pickup': {
        const data = event.data as SkullPickupData
        const serverName = getServerName(event.server_id)
        const enemyName = data.team === 1 ? 'Blue' : 'Red'
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        addActivity('info', `${data.player_name}${COLOR_RESET} picked up a ${enemyName} skull (${data.count})`, {
          player, serverId: event.server_id, serverName, activityType: 'skull_pickup', team: data.team,
        })
        break
      }
      case 'skull_score': {
        const data = event.data as SkullScoreData
        const serverName = getServerName(event.server_id)
        const teamName = data.team === 1 ? 'Red' : 'Blue'
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        addActivity('info', `${data.player_name}${COLOR_RESET} scored ${data.skulls} skull${data.skulls !== 1 ? 's' : ''} for ${teamName}`, {
          player, serverId: event.server_id, serverName, activityType: 'skull_score', team: data.team,
        })
        break
      }
      case 'team_change': {
        const data = event.data as TeamChangeData
        const serverName = getServerName(event.server_id)
        const teamNames = ['Free', 'Red', 'Blue', 'Spectator']
        const oldTeam = teamNames[data.old_team] || 'Unknown'
        const newTeam = teamNames[data.new_team] || 'Unknown'
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        addActivity('info', `${data.player_name}${COLOR_RESET}: ${oldTeam} → ${newTeam}`, {
          player, serverId: event.server_id, serverName, activityType: 'team_change',
        })
        break
      }
      case 'say': {
        const data = event.data as SayData
        const serverName = getServerName(event.server_id)
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        addActivity('chat', `${data.player_name}${COLOR_RESET}: ${data.message}`, { player, serverId: event.server_id, serverName })
        break
      }
      case 'say_team': {
        const data = event.data as SayTeamData
        const serverName = getServerName(event.server_id)
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        addActivity('chat', `(team) ${data.player_name}${COLOR_RESET}: ${data.message}`, { player, serverId: event.server_id, serverName })
        break
      }
      case 'tell': {
        const data = event.data as TellData
        const serverName = getServerName(event.server_id)
        const cleanName = cleanQ3Name(data.from_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.from_name, cleanName, playerId: data.from_player_id, ...botInfo }
        addActivity('chat', `${data.from_name}${COLOR_RESET} -> ${data.to_name}${COLOR_RESET}: ${data.message}`, { player, serverId: event.server_id, serverName })
        break
      }
      case 'say_rcon': {
        const data = event.data as SayRconData
        const serverName = getServerName(event.server_id)
        addActivity('chat', `server: ${data.message}`, { serverId: event.server_id, serverName })
        break
      }
      case 'award': {
        const data = event.data as AwardData
        const serverName = getServerName(event.server_id)
        const cleanName = cleanQ3Name(data.player_name)
        const botInfo = getPlayerBotInfo(event.server_id, cleanName)
        const player = { name: data.player_name, cleanName, playerId: data.player_id, ...botInfo }
        let message: string
        let victim: ActivityPlayer | undefined
        switch (data.award_type) {
          case 'impressive': message = `${data.player_name}${COLOR_RESET} was impressive!`; break
          case 'excellent': message = `${data.player_name}${COLOR_RESET} was excellent!`; break
          case 'humiliation':
            if (data.victim_name) {
              const victimCleanName = cleanQ3Name(data.victim_name)
              const victimBotInfo = getPlayerBotInfo(event.server_id, victimCleanName)
              victim = { name: data.victim_name, cleanName: victimCleanName, playerId: data.victim_player_id, ...victimBotInfo }
              message = `${data.player_name}${COLOR_RESET} humiliated ${data.victim_name}${COLOR_RESET}!`
            } else {
              message = `${data.player_name}${COLOR_RESET} earned a humiliation!`
            }
            break
          case 'defend': {
            const gameType = getServerGameType(event.server_id)
            const teamColor = data.team === 1 ? 'Red' : data.team === 2 ? 'Blue' : null
            if (gameType === 'overload') {
              message = teamColor ? `${data.player_name}${COLOR_RESET} defended the ${teamColor} obelisk!` : `${data.player_name}${COLOR_RESET} defended the obelisk!`
            } else if (gameType === 'harvester') {
              message = `${data.player_name}${COLOR_RESET} defended the obelisk!`
            } else if (gameType === '1fctf') {
              message = `${data.player_name}${COLOR_RESET} defended the flag!`
            } else if (teamColor) {
              message = `${data.player_name}${COLOR_RESET} defended the ${teamColor} flag!`
            } else {
              message = `${data.player_name}${COLOR_RESET} defended the flag!`
            }
            break
          }
          case 'assist': message = `${data.player_name}${COLOR_RESET} assisted a capture!`; break
          default: message = `${data.player_name}${COLOR_RESET} earned ${data.award_type}!`
        }
        addActivity('info', message, {
          player, serverId: event.server_id, serverName,
          activityType: 'award', awardType: data.award_type, victim,
        })
        break
      }
    }
  }, [addActivity, getServerName, getServerGameType, getPlayerBotInfo, refreshObeliskAttackerState])

  const wsUrl = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`
  const { isConnected, send: wsSend } = useWebSocket(wsUrl, handleEvent)

  // Subscribe to "server" only when the drawer is closed (most of the
  // time on most clients) and add "activity" when it's open. The hook
  // caches the payload and replays on reconnect so the wire stays
  // honest across socket flaps. Initial render fires before the socket
  // is open — that's fine, the cached payload is sent in onopen.
  useEffect(() => {
    wsSend({
      type: 'subscribe',
      categories: activityDrawerOpen ? ['server', 'activity'] : ['server'],
    })
  }, [activityDrawerOpen, wsSend])

  // Periodic /api/servers fetch (auth-aware: re-runs on token change so
  // manageable_by_me reflects current login state).
  useEffect(() => {
    let cancelled = false
    async function fetchServers(initial: boolean) {
      try {
        const headers: HeadersInit = auth.token ? { Authorization: `Bearer ${auth.token}` } : {}
        const res = await fetch('/api/servers', { headers })
        const serverList: Server[] = await res.json()
        if (cancelled) return

        const livenessMap = new Map<number, 'live' | 'stale' | 'offline'>()
        const manageableMap = new Map<number, boolean>()
        const visibleIds = new Set<number>()
        serverList.forEach((s) => {
          if (s.liveness) livenessMap.set(s.id, s.liveness)
          if (s.manageable_by_me) manageableMap.set(s.id, true)
          visibleIds.add(s.id)
        })
        setLiveness(livenessMap)
        setManageable(manageableMap)
        visibleServerIdsRef.current = visibleIds

        if (initial) {
          const statusMap = new Map<number, ServerStatus>()
          await Promise.all(
            serverList.map(async (server) => {
              try {
                const r = await fetch(`/api/servers/${server.id}/status`)
                if (r.ok) {
                  const status: ServerStatus = await r.json()
                  statusMap.set(server.id, status)
                }
              } catch (e) {
                console.error(`Failed to fetch status for server ${server.id}:`, e)
              }
            }),
          )
          if (cancelled) return
          setServers(statusMap)
        } else {
          // Subsequent polls: prune any cached statuses for servers that
          // just dropped off the visible list.
          setServers((prev) => {
            const next = new Map<number, ServerStatus>()
            prev.forEach((status, id) => { if (visibleIds.has(id)) next.set(id, status) })
            return next
          })
        }
      } catch (e) {
        console.error('Failed to fetch servers:', e)
      } finally {
        if (initial && !cancelled) setLoading(false)
      }
    }

    fetchServers(true)
    const interval = setInterval(() => fetchServers(false), 30000)
    return () => { cancelled = true; clearInterval(interval) }
  }, [auth.token])

  // Forced password change — surfaces regardless of route.
  useEffect(() => {
    if (auth.isAuthenticated && auth.passwordChangeRequired) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setShowPasswordChange(true)
    }
  }, [auth.isAuthenticated, auth.passwordChangeRequired])

  const showPlayer = useCallback((playerName: string, _cleanName: string, playerId?: number) => {
    if (playerId) setSelectedPlayer({ name: playerName, playerId })
  }, [])

  const closePlayer = useCallback(() => setSelectedPlayer(null), [])

  // Bumped by the modal's "View full profile" CTA so pages can react to
  // drill-in intent independently of URL change.
  const [drillInVersion, setDrillInVersion] = useState(0)
  const notifyDrillIn = useCallback(() => setDrillInVersion((v) => v + 1), [])

  const toggleActivityDrawer = useCallback(() => setActivityDrawerOpen((v) => !v), [])

  const value = useMemo<LiveDataValue>(() => ({
    servers, liveness, manageable, activities, newPlayers, loading, isConnected,
    obeliskAttackersByServer,
    activeHumanPlayersCount, activeServersCount,
    activityDrawerOpen, setActivityDrawerOpen, toggleActivityDrawer,
    selectedPlayer, showPlayer, closePlayer,
    drillInVersion, notifyDrillIn,
    showPasswordChange, setShowPasswordChange,
    commandPaletteOpen, setCommandPaletteOpen,
  }), [
    servers, liveness, manageable, activities, newPlayers, loading, isConnected,
    obeliskAttackersByServer,
    activeHumanPlayersCount, activeServersCount,
    activityDrawerOpen, toggleActivityDrawer,
    selectedPlayer, showPlayer, closePlayer,
    drillInVersion, notifyDrillIn,
    showPasswordChange,
    commandPaletteOpen,
  ])

  return <LiveDataContext.Provider value={value}>{children}</LiveDataContext.Provider>
}
