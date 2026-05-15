// Canonical 1FCTF (One-Flag CTF) snapshot for the docs section.
//
// New tooltip surface this round:
//   - Neutral flag indicator in the Scoreboard's center slot. The
//     same row-carrier element used in CTF carries a different help
//     text variant when flagCarrier === 'neutral' (already wired in
//     PlayerRows.tsx).
//
// Snapshot state: neutral flag is carried by a red-team player, so
// the indicator drifts toward red and the carrier shows a row-level
// neutral flag icon. (The neutral flag has three meaningful states:
// at-base, carried-toward-X, dropped — we pick "carried" here so the
// drift indicator and the row carrier are both demonstrated.)
import type { ServerStatus } from '../../../../types'

export const ONE_FCTF_SNAPSHOT: ServerStatus = {
  server_id: -5,
  source: 'hub',
  key: '1fctf',
  address: 'trinity.run:27964',
  map: 'hal_palindrome',
  game_type: '1fctf',
  game_time_ms: 482000, // ~8:02 elapsed
  max_clients: 12,
  human_count: 6,
  bot_count: 2,
  online: true,
  last_updated: '2026-05-15T00:14:15.614Z',
  match_state: 'active',
  team_scores: { red: 1, blue: 2 },
  obj_status: {
    mode: '1fctf',
    // Neutral flag carried by red — drifts toward red on the indicator,
    // and the carrier (client_num 0, on red team) shows a row-level
    // neutral flag icon.
    neutral: 2, // 0=base, 2=carried-by-red, 3=carried-by-blue, 4=dropped
    neutral_carrier: 0,
  },
  players: [
    // Red team
    {
      client_num: 0,
      name: 'kreshok',
      clean_name: 'kreshok',
      score: 12,
      ping: 36,
      is_bot: false,
      is_verified: true,
      team: 1,
      excellents: 1,
      player_id: 501,
      model: 'ranger/red',
    },
    {
      client_num: 1,
      name: 'evergreen',
      clean_name: 'evergreen',
      score: 9,
      ping: 41,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 1,
      defends: 1,
      player_id: 502,
      model: 'mynx/red',
    },
    {
      client_num: 2,
      name: 'predzy',
      clean_name: 'predzy',
      score: 7,
      ping: 28,
      is_bot: false,
      is_verified: true,
      team: 1,
      assists: 1,
      player_id: 503,
      model: 'sorlag/red',
    },
    {
      client_num: 3,
      name: 'Major',
      clean_name: 'Major',
      score: 5,
      ping: 0,
      is_bot: true,
      skill: 3,
      team: 1,
      model: 'major/red',
    },
    // Blue team
    {
      client_num: 4,
      name: 'spudly',
      clean_name: 'spudly',
      score: 14,
      ping: 45,
      is_bot: false,
      is_verified: true,
      team: 2,
      captures: 1,
      defends: 1,
      excellents: 1,
      player_id: 504,
      model: 'bones/blue',
    },
    {
      client_num: 5,
      name: 'foxtrot',
      clean_name: 'foxtrot',
      score: 11,
      ping: 39,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 2,
      captures: 1,
      player_id: 505,
      model: 'orbb/blue',
    },
    {
      client_num: 6,
      name: 'prodice',
      clean_name: 'prodice',
      score: 6,
      ping: 52,
      is_bot: false,
      is_verified: true,
      team: 2,
      player_id: 506,
      model: 'visor/blue',
    },
    {
      client_num: 7,
      name: 'Klesk',
      clean_name: 'Klesk',
      score: 3,
      ping: 0,
      is_bot: true,
      skill: 3,
      team: 2,
      model: 'klesk/blue',
    },
    // No spectators on this card — variety across the docs section.
  ],
  server_vars: {
    capturelimit: '5',
    timelimit: '20',
    sv_hostname: 'Trinity 1FCTF',
    g_gametype: '5',
    g_gameplay: '0',
    g_movement: '0',
    g_redteam: 'Red',
    g_blueteam: 'Blue',
    mapname: 'hal_palindrome',
  },
}
