// Canonical Harvester snapshot for the docs section.
//
// New tooltip surfaces this round:
//   - Team skull-carry chip in the Scoreboard slots.
//   - Row-level skull-carry indicator on players currently holding skulls.
//
// Snapshot state:
//   - Red team is holding 3 enemy skulls — split across TWO carriers
//     (jam: 2 + Viper: 1) so the team chip's count is visibly the sum
//     of two row indicators, not a single carrier's number.
//   - Blue team is holding 2 enemy skulls (Twiztt has both).
//   - Team scores 4–3 (red leads on deliveries).
//   - Uses a Team Arena model (*james) on Twiztt as the TA variety beat.
//   - Spectator count bumped to 3 (the prior modes had 0–1; varying
//     by card so the strip isn't always uniform).
//
// Mechanics referenced in the tooltips (corrections from the editorial
// pass): skulls spawn at the central skull generator on each frag,
// not at the kill site; carriers can hold up to 5 at once; picking up
// your own team's skulls denies them to the enemy.
import type { ServerStatus } from '../../../../types'

export const HARVESTER_SNAPSHOT: ServerStatus = {
  server_id: -7,
  source: 'hub',
  key: 'harvester',
  address: 'trinity.run:27966',
  map: 'czq3p61ctf1',
  game_type: 'harvester',
  game_time_ms: 685000, // ~11:25 elapsed
  max_clients: 12,
  human_count: 7,
  bot_count: 2,
  online: true,
  last_updated: '2026-05-15T00:14:15.614Z',
  match_state: 'active',
  team_scores: { red: 4, blue: 3 },
  obj_status: {
    mode: 'harvester',
    red_skulls: 3,  // jam (2) + Viper (1)
    blue_skulls: 2, // Twiztt (2)
  },
  players: [
    // Red team
    {
      client_num: 0,
      name: 'Swisha',
      clean_name: 'Swisha',
      score: 14,
      ping: 34,
      is_bot: false,
      is_verified: true,
      team: 1,
      skulls_delivered: 2,
      excellents: 1,
      player_id: 701,
      model: 'razor/red',
    },
    {
      client_num: 1,
      name: 'jam',
      clean_name: 'jam',
      score: 9,
      ping: 42,
      is_bot: false,
      is_verified: true,
      team: 1,
      skulls_carrying: 2, // active carrier #1 on red
      skulls_delivered: 1,
      player_id: 702,
      model: 'keel/red',
    },
    {
      client_num: 2,
      name: 'Viper',
      clean_name: 'Viper',
      score: 7,
      ping: 28,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 1,
      skulls_carrying: 1, // active carrier #2 on red — chip sums to 3
      player_id: 703,
      model: 'ranger/red',
    },
    {
      client_num: 3,
      name: 'Sarge',
      clean_name: 'Sarge',
      score: 5,
      ping: 0,
      is_bot: true,
      skill: 3,
      team: 1,
      model: 'sarge/red',
    },
    // Blue team
    {
      client_num: 4,
      name: 'VoO',
      clean_name: 'VoO',
      score: 12,
      ping: 39,
      is_bot: false,
      is_verified: true,
      team: 2,
      skulls_delivered: 1,
      player_id: 704,
      model: 'uriel/blue',
    },
    {
      client_num: 5,
      name: 'Twiztt',
      clean_name: 'Twiztt',
      score: 11,
      ping: 47,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 2,
      skulls_carrying: 2, // sole carrier on blue
      skulls_delivered: 2,
      impressives: 1,
      player_id: 705,
      // Team Arena model — engine prefixes TA heads with `*`,
      // PlayerPortrait strips it before resolving the asset path.
      model: '*james/blue',
    },
    {
      client_num: 6,
      name: 'riv',
      clean_name: 'riv',
      score: 8,
      ping: 51,
      is_bot: false,
      // unverified — sparse variety beat.
      team: 2,
      player_id: 706,
      model: 'bones/blue',
    },
    {
      client_num: 7,
      name: 'Doom',
      clean_name: 'Doom',
      score: 4,
      ping: 0,
      is_bot: true,
      skill: 4,
      team: 2,
      model: 'doom/blue',
    },
    // Spectators — three this card, varying from the 1-spec pattern of
    // earlier modes.
    {
      client_num: 8,
      name: 'Phantom',
      clean_name: 'Phantom',
      score: 0,
      ping: 36,
      is_bot: false,
      is_verified: true,
      team: 3,
      player_id: 707,
    },
    {
      client_num: 9,
      name: 'Tempest',
      clean_name: 'Tempest',
      score: 0,
      ping: 44,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 3,
      player_id: 708,
    },
    {
      client_num: 10,
      name: 'spawn',
      clean_name: 'spawn',
      score: 0,
      ping: 51,
      is_bot: false,
      is_verified: true,
      team: 3,
      player_id: 709,
    },
  ],
  server_vars: {
    capturelimit: '8',
    timelimit: '20',
    sv_hostname: 'Trinity Harvester',
    g_gametype: '7',
    g_gameplay: '0',
    g_movement: '0',
    g_redteam: 'Red',
    g_blueteam: 'Blue',
    mapname: 'czq3p61ctf1',
  },
}
