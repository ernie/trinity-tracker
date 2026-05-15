// Canonical Overload snapshot for the docs section.
//
// New tooltip surfaces this round:
//   - ObeliskHPIndicator (in Scoreboard red/blue slots).
//   - Row-level obelisk-attacker reticle.
//
// Snapshot state:
//   - Blue obelisk is at 1400/2500 HP and under attack — its bar
//     is highlighted, the row-attacker reticle shows on the attacker.
//   - Red obelisk is at full HP (2500/2500) — not under attack.
//   - Team scores 1–0 (red previously destroyed blue once and the
//     blue obelisk respawned); the `obelisks_destroyed: 1` count on
//     the destroyer surfaces the obelisk medal in their row.
//
// The "currently attacking" state is supplied by passing an
// attackersOverride Set to the ServerCardDemo wrapper — the live
// ServerCard reads attackingSlots from LiveDataContext, but the
// snapshot needs a static way to demonstrate the indicator.
// dread (client_num 0, red team) is the attacker in this snapshot.
import type { ServerStatus } from '../../../../types'

export const OVERLOAD_SNAPSHOT: ServerStatus = {
  server_id: -6,
  source: 'hub',
  key: 'overload',
  address: 'trinity.run:27965',
  map: 'mpteam4',
  game_type: 'overload',
  game_time_ms: 738000, // ~12:18 elapsed
  max_clients: 12,
  human_count: 6,
  bot_count: 2,
  online: true,
  last_updated: '2026-05-15T00:14:15.614Z',
  match_state: 'active',
  team_scores: { red: 1, blue: 0 },
  obelisk_health_max: 2500,
  obj_status: {
    mode: 'overload',
    red_obelisk_hp: 2500, // full
    blue_obelisk_hp: 1400, // ~56%, under attack from red
  },
  players: [
    // Red team (team: 1) — dread is currently attacking blue's
    // obelisk (its client_num 0 must be in the attackersOverride Set
    // supplied by DocsPlay).
    {
      client_num: 0,
      name: 'Dread',
      clean_name: 'Dread',
      score: 14,
      ping: 33,
      is_bot: false,
      is_verified: true,
      team: 1,
      excellents: 1,
      player_id: 601,
      model: 'visor/red',
    },
    {
      client_num: 1,
      name: 'Monstr',
      clean_name: 'Monstr',
      score: 22,
      ping: 27,
      is_bot: false,
      is_verified: true,
      is_admin: true, // the rare admin variety
      team: 1,
      excellents: 2,
      impressives: 1,
      obelisks_destroyed: 1, // the prior destruction → 'obelisk' medal
      player_id: 602,
      model: 'keel/red',
    },
    {
      client_num: 2,
      name: 'Sane',
      clean_name: 'Sane',
      score: 9,
      ping: 44,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 1,
      player_id: 603,
      model: 'ranger/red',
    },
    {
      client_num: 3,
      name: 'TankJr',
      clean_name: 'TankJr',
      score: 7,
      ping: 0,
      is_bot: true,
      skill: 4,
      team: 1,
      model: 'tankjr/red',
    },
    // Blue team (team: 2)
    {
      client_num: 4,
      name: 'Danillzin',
      clean_name: 'Danillzin',
      score: 16,
      ping: 38,
      is_bot: false,
      is_verified: true,
      team: 2,
      excellents: 1,
      defends: 1,
      player_id: 604,
      model: 'razor/blue',
    },
    {
      client_num: 5,
      name: 'pred',
      clean_name: 'pred',
      score: 11,
      ping: 49,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 2,
      player_id: 605,
      model: 'bones/blue',
    },
    {
      client_num: 6,
      name: 'mz',
      clean_name: 'mz',
      score: 5,
      ping: 31,
      is_bot: false,
      is_verified: true,
      team: 2,
      player_id: 606,
      model: 'orbb/blue',
    },
    {
      client_num: 7,
      name: 'Bitterman',
      clean_name: 'Bitterman',
      score: 4,
      ping: 0,
      is_bot: true,
      skill: 3,
      team: 2,
      model: 'bitterman/blue',
    },
    // Spectators
    {
      client_num: 8,
      name: 'Vegeta',
      clean_name: 'Vegeta',
      score: 0,
      ping: 47,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 3,
      player_id: 607,
    },
    {
      client_num: 9,
      name: 'Vortex',
      clean_name: 'Vortex',
      score: 0,
      ping: 53,
      is_bot: false,
      is_verified: true,
      team: 3,
      player_id: 608,
    },
  ],
  server_vars: {
    capturelimit: '8',
    timelimit: '20',
    sv_hostname: 'Trinity Overload',
    g_gametype: '6',
    g_gameplay: '0',
    g_movement: '0',
    g_redteam: 'Red',
    g_blueteam: 'Blue',
    mapname: 'mpteam4',
  },
}

// The attacker Set lives alongside the snapshot — when DocsPlay
// renders this card, it must pass this Set as attackersOverride so
// the under-attack indicator + row reticle resolve.
export const OVERLOAD_ATTACKERS: Set<number> = new Set([0]) // dread
