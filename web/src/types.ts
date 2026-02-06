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
  name: string
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
  match_state?: 'waiting' | 'warmup' | 'active' | 'intermission'
  warmup_remaining?: number // milliseconds remaining in warmup
}

export interface Server {
  id: number
  name: string
  address: string
  log_path?: string
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
  server_name: string
  map_name: string
  game_type: string
  started_at: string
  ended_at?: string
  exit_reason?: string
  players: MatchPlayerSummary[]
  red_score?: number
  blue_score?: number
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
  password_change_required: boolean
  created_at: string
  last_login: string | null
}

export interface AccountProfile {
  user: User
  player?: PlayerProfile
  guids?: PlayerGUID[]
}

export interface VerifiedPlayer {
  player_id: number
  clean_name: string
  is_admin: boolean
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
  server_name: string
  joined_at: string
  left_at?: string
  duration_seconds?: number
  ip_address?: string
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
