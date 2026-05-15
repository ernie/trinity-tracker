// Canonical CTF (Capture the Flag) snapshot for the docs section.
//
// New tooltip surfaces this round:
//   - Red / blue flag indicators in the Scoreboard slots.
//   - Row-level flag-carrier icon (FlagIcon next to a player's name).
//
// State is chosen so the two flag indicators show DIFFERENT statuses
// (dropped vs taken) — two carriers both running would just show the
// same indicator twice. One row-carrier is enough to demo the carrier
// icon; the dropped state demonstrates the "lies on the ground" status
// in parallel.
import type { ServerStatus } from '../../../../types'

export const CTF_SNAPSHOT: ServerStatus = {
  server_id: -4,
  source: 'hub',
  key: 'ctf-ta',
  address: 'trinity.run:27963',
  map: 'q3wcp10',
  game_type: 'ctf',
  game_time_ms: 612000, // ~10:12 elapsed
  max_clients: 12,
  human_count: 6,
  bot_count: 2,
  online: true,
  last_updated: '2026-05-14T23:14:15.614Z',
  match_state: 'active',
  team_scores: { red: 2, blue: 1 }, // 2 captures vs 1
  obj_status: {
    mode: 'ctf',
    red: 2,            // red flag dropped — lies on the ground; no carrier
    blue: 1,           // blue flag taken (carried by client_num 0, on red team)
    blue_carrier: 0,
  },
  players: [
    // Red team (team: 1)
    {
      client_num: 0,
      name: 'MaverickQc',
      clean_name: 'MaverickQc',
      score: 18,
      ping: 33,
      is_bot: false,
      is_verified: true,
      team: 1,
      captures: 1,
      assists: 2,
      excellents: 2,
      player_id: 401,
      model: 'razor/red',
    },
    {
      client_num: 1,
      name: 'agentbob',
      clean_name: 'agentbob',
      score: 12,
      ping: 46,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 1,
      defends: 2,
      excellents: 1,
      player_id: 402,
      model: 'bones/red',
    },
    {
      client_num: 2,
      name: 'kapet',
      clean_name: 'kapet',
      score: 8,
      ping: 51,
      is_bot: false,
      // unverified — sparse variety per editorial guideline.
      team: 1,
      defends: 1,
      player_id: 403,
      model: 'uriel/red',
    },
    {
      client_num: 3,
      name: 'Hunter',
      clean_name: 'Hunter',
      score: 5,
      ping: 0,
      is_bot: true,
      skill: 3,
      team: 1,
      model: 'hunter/red',
    },
    // Blue team (team: 2)
    {
      client_num: 4,
      name: 'psygib',
      clean_name: 'psygib',
      score: 14,
      ping: 29,
      is_bot: false,
      is_verified: true,
      team: 2,
      captures: 1,
      assists: 1,
      excellents: 1,
      player_id: 404,
      model: 'bitterman/blue',
    },
    {
      client_num: 5,
      name: 'lostcontrol',
      clean_name: 'lostcontrol',
      score: 10,
      ping: 38,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 2,
      defends: 1,
      assists: 1,
      player_id: 405,
      model: 'klesk/blue',
    },
    {
      client_num: 6,
      name: 'fizz',
      clean_name: 'fizz',
      score: 7,
      ping: 44,
      is_bot: false,
      is_verified: true,
      team: 2,
      player_id: 406,
      model: 'major/blue',
    },
    {
      client_num: 7,
      name: 'Crash',
      clean_name: 'Crash',
      score: 4,
      ping: 0,
      is_bot: true,
      skill: 2,
      team: 2,
      model: 'crash/blue',
    },
    // Spectator
    {
      client_num: 8,
      name: 'ryan',
      clean_name: 'ryan',
      score: 0,
      ping: 42,
      is_bot: false,
      is_verified: true,
      team: 3,
      player_id: 407,
    },
  ],
  server_vars: {
    fraglimit: '100',
    timelimit: '20',
    capturelimit: '5',
    sv_hostname: 'Trinity CTF',
    g_gametype: '4',
    g_gameplay: '0',
    g_movement: '0',
    g_redteam: 'Red',
    g_blueteam: 'Blue',
    mapname: 'q3wcp10',
  },
}
