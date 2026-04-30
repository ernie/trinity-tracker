export interface Player {
  client_num: number
  name: string
  clean_name: string
  score: number
  ping: number
  is_bot: boolean
  skill?: number  // bot skill level (1-5), 0 or undefined if human
  team?: number
  joined_at?: string
  impressives?: number
  excellents?: number
  humiliations?: number
  defends?: number
  captures?: number
  assists?: number
  player_id?: number
  model?: string
  is_vr?: boolean
  is_verified?: boolean
  is_admin?: boolean
}

export interface TeamScores {
  red: number
  blue: number
}

export interface FlagStatus {
  red: number         // 0=at base, 1=taken, 2=dropped
  red_carrier: number // client_num of carrier, or -1
  blue: number
  blue_carrier: number
}

export interface ServerStatus {
  server_id: number
  source: string
  key: string
  address: string
  map: string
  game_type: string
  game_time_ms: number
  max_clients: number
  players: Player[]
  human_count: number
  bot_count: number
  online: boolean
  last_updated: string
  team_scores?: TeamScores
  flag_status?: FlagStatus
  server_vars?: Record<string, string>
  match_state?: 'waiting' | 'warmup' | 'active' | 'overtime' | 'intermission'
  warmup_remaining?: number // milliseconds remaining in warmup
}

export interface Server {
  id: number
  source: string
  key: string
  address: string
  active: boolean
  online?: boolean
  liveness?: 'live' | 'stale' | 'offline'
}

export type EventType =
  | 'player_join'
  | 'player_leave'
  | 'server_update'
  | 'match_start'
  | 'match_end'
  | 'frag'
  | 'flag_capture'
  | 'flag_taken'
  | 'flag_return'
  | 'flag_drop'
  | 'obelisk_destroy'
  | 'skull_score'
  | 'team_change'
  | 'say'
  | 'say_team'
  | 'tell'
  | 'say_rcon'
  | 'award'

export interface WSEvent {
  event: EventType
  server_id: number
  timestamp: string
  data: unknown
}

export interface PlayerJoinData {
  player: Player
  player_id?: number
}

export interface PlayerLeaveData {
  player_name: string
  player_id?: number
}

export interface MatchStartData {
  map: string
  game_type: string
}

export interface FlagCaptureData {
  client_num: number
  player_name: string
  team: number
  player_id?: number
}

export interface FlagTakenData {
  client_num: number
  player_name: string
  team: number
  player_id?: number
}

export interface FlagReturnData {
  client_num: number
  player_name: string
  team: number
  player_id?: number
}

export interface FlagDropData {
  client_num: number
  player_name: string
  team: number
  player_id?: number
}

export interface ObeliskDestroyData {
  attacker_name: string
  team: number
  player_id?: number
}

export interface SkullScoreData {
  player_name: string
  team: number
  skulls: number
  player_id?: number
}

export interface TeamChangeData {
  player_name: string
  old_team: number
  new_team: number
  player_id?: number
}

export interface SayData {
  client_num: number
  player_name: string
  message: string
  player_id?: number
}

export interface SayTeamData {
  client_num: number
  player_name: string
  message: string
  player_id?: number
}

export interface TellData {
  from_client_num: number
  to_client_num: number
  from_name: string
  to_name: string
  message: string
  from_player_id?: number
  to_player_id?: number
}

export interface SayRconData {
  message: string
}

export interface AwardData {
  client_num: number
  player_name: string
  award_type: 'impressive' | 'excellent' | 'humiliation' | 'defend' | 'assist'
  team?: number // 1=Red, 2=Blue
  player_id?: number
  victim_name?: string
  victim_player_id?: number
}

export interface ActivityPlayer {
  name: string
  cleanName: string
  playerId?: number
  isBot?: boolean
  isVR?: boolean
  isVerified?: boolean
  isAdmin?: boolean
  skill?: number
}

export type ActivityType =
  | 'flag_capture'
  | 'flag_taken'
  | 'flag_return'
  | 'flag_drop'
  | 'award'
  | 'match_start'
  | 'obelisk_destroy'
  | 'skull_score'
  | 'team_change'

export interface ActivityItem {
  id: number
  timestamp: Date
  type: 'join' | 'leave' | 'info' | 'chat'
  message: string
  player?: ActivityPlayer
  serverId?: number
  serverName?: string
  activityType?: ActivityType
  team?: number // 1=Red, 2=Blue
  awardType?: 'impressive' | 'excellent' | 'humiliation' | 'defend' | 'assist'
  mapName?: string // for match_start events
  victim?: ActivityPlayer // for humiliation awards
}

export interface MatchPlayerSummary {
  player_id: number
  name: string
  clean_name: string
  frags: number
  deaths: number
  completed: boolean
  is_bot: boolean
  is_vr?: boolean
  is_verified?: boolean
  is_admin?: boolean
  skill?: number
  score?: number
  team?: number
  model?: string
  impressives?: number
  excellents?: number
  humiliations?: number
  defends?: number
  victories?: number
  captures?: number
  assists?: number
}

export interface MatchSummary {
  id: number
  server_id: number
  server_key: string
  server_active: boolean
  source: string
  map_name: string
  game_type: string
  started_at: string
  ended_at?: string
  exit_reason?: string
  players: MatchPlayerSummary[]
  red_score?: number
  blue_score?: number
  demo_url?: string
  movement?: string
  gameplay?: string
}

// Auth types
export interface AuthState {
  isAuthenticated: boolean
  username: string | null
  token: string | null
  isAdmin: boolean
  playerId: number | null
  passwordChangeRequired: boolean
}

export interface LoginCredentials {
  username: string
  password: string
}

export interface User {
  id: number
  username: string
  is_admin: boolean
  player_id: number | null
  player_name?: string | null
  password_change_required: boolean
  created_at: string
  last_login: string | null
}

export interface AccountProfile {
  user: User
  player?: PlayerProfile
  guids?: PlayerGUID[]
}

export interface RconCommand {
  id: number
  command: string
  output: string
  timestamp: Date
  serverName: string
}

// Player stats types
export interface AggregatedStats {
  matches: number
  completed_matches: number
  uncompleted_matches: number
  frags: number
  deaths: number
  kd_ratio: number
  captures: number
  flag_returns: number
  assists: number
  impressives: number
  excellents: number
  humiliations: number
  defends: number
  victories: number
}

export interface PlayerGUID {
  id: number
  player_id: number
  guid: string
  name: string
  clean_name: string
  first_seen: string
  last_seen: string
  is_vr?: boolean
}

export interface PlayerSession {
  id: number
  server_id: number
  server_source: string
  server_key: string
  joined_at: string
  left_at?: string
  duration_seconds?: number
  ip_address?: string
  client_engine?: string
  client_version?: string
}

export interface AdminSession {
  id: number
  server_id: number
  server_source: string
  server_key: string
  player_id: number
  player_name: string
  player_clean_name: string
  joined_at: string
  left_at?: string
  duration_seconds?: number
  ip_address?: string
  client_engine?: string
  client_version?: string
}

export interface PlayerName {
  name: string
  clean_name: string
  first_seen: string
  last_seen: string
}

export interface PlayerProfile {
  id: number
  name: string
  clean_name: string
  first_seen: string
  last_seen: string
  total_playtime_seconds: number
  is_bot?: boolean
  is_vr?: boolean
  is_verified?: boolean
  is_admin?: boolean
  model?: string
  skill?: number
  guids?: PlayerGUID[]
}

export interface PlayerStatsResponse {
  player: PlayerProfile
  period: string
  period_start?: string
  period_end?: string
  stats: AggregatedStats
  names: PlayerName[]
}

export type TimePeriod = 'all' | 'day' | 'week' | 'month' | 'year'

export type LeaderboardCategory =
  | 'frags'
  | 'deaths'
  | 'kd_ratio'
  | 'captures'
  | 'flag_returns'
  | 'matches'
  | 'assists'
  | 'impressives'
  | 'excellents'
  | 'humiliations'
  | 'defends'
  | 'victories'

export interface LeaderboardEntry {
  rank: number
  player: PlayerProfile
  total_frags: number
  total_deaths: number
  total_matches: number
  completed_matches: number
  uncompleted_matches: number
  kd_ratio: number
  captures: number
  flag_returns: number
  assists: number
  impressives: number
  excellents: number
  humiliations: number
  defends: number
  victories: number
}

export interface LeaderboardResponse {
  category: LeaderboardCategory
  period: TimePeriod
  period_start?: string
  period_end?: string
  entries: LeaderboardEntry[]
}

// Trinity engine module — surface used by demo player and play page.
// Built from emscripten + custom additions in trinity-engine's loader.js.
export interface EngineModule {
  abort: () => void
  shutdown: () => void
  pauseMainLoop: () => void
  _exit: (code: number) => void
  ccall(name: string, returnType: 'string', argTypes: string[], args: unknown[]): string | null
  ccall(name: string, returnType: null, argTypes: string[], args: unknown[]): void
  onNextFrame: (cb: () => void) => void
}

// Self-service collector onboarding.

// SourceStatus mirrors the sources.status enum on the server. Drives
// the per-source-card UI in the My Servers drawer.
export type SourceStatus = 'pending' | 'active' | 'rejected' | 'left' | 'revoked'

export interface MySourceServer {
  key: string
  address: string
  active: boolean
}

// MySourceEntry is one source the caller owns. A user may own
// multiple — the recommended pattern is one source per host so each
// physical machine gets its own creds and per-collector controls.
export interface MySourceEntry {
  source: string
  status: SourceStatus
  purpose?: string
  rejection_reason?: string
  // Active-only fields (collector-reported via heartbeat):
  version?: string
  demo_base_url?: string
  last_heartbeat_at?: string
  servers?: MySourceServer[]
}

// MySources is the JSON envelope returned by GET /api/sources/mine.
// has_pending lets the header button decide its label without the
// caller having to scan the array.
export interface MySources {
  sources: MySourceEntry[]
  has_pending: boolean
}

// PendingRequest is one row of the admin pending list. The collector's
// URL is intentionally absent — it arrives via heartbeat once the
// collector connects, so asking for it up front would be redundant.
export interface PendingRequest {
  source: string
  owner_user_id: number
  owner_username: string
  requested_purpose: string
  submitted_at: string
}
