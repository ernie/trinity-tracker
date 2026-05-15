// Canonical 1v1 (Tournament) snapshot for the docs section.
//
// Captured from /api/servers/2/status then hand-edited to:
//   - Replace the bot pair with two verified humans so the two-player
//     side-by-side card (Duelists.tsx — internal component name only,
//     never user-facing) demonstrates how the shared badges /
//     portraits / names render at the larger size used here.
//   - Asymmetric scores so the layout reads clearly as "side-by-side
//     scoreboard for the two opponents," not a degenerate player table.
//   - A spectator queue so the SpectatorStrip's "next plays the winner"
//     tooltip has something to anchor on.
//   - Fresh names — no overlap with the FFA snapshot.
//   - Realistic badge distribution: mostly verified users + verified
//     VR; admins are rare in real communities so we don't lean on them.
//
// Tournament has no fraglimit by default (only timelimit); the
// card__limits cell renders as `9:56 / 10m` without a `Limit:` prefix.
// The LIMITS_HELP tooltip still describes the score-limit concept for
// modes that do use one.
import type { ServerStatus } from '../../../../types'

export const ONE_V_ONE_SNAPSHOT: ServerStatus = {
  server_id: -2,
  source: 'hub',
  key: '1v1',
  address: 'trinity.run:27961',
  map: 'q3tourney2',
  game_type: '1v1',
  game_time_ms: 596000, // ~9:56 elapsed
  max_clients: 8,
  human_count: 5,
  bot_count: 0,
  online: true,
  last_updated: '2026-05-14T23:14:15.614Z',
  match_state: 'active',
  // First two players (by score order) become the side-by-side
  // opponents; the rest are spectators (team: 3).
  players: [
    {
      client_num: 0,
      name: 'rapha',
      clean_name: 'rapha',
      score: 5,
      ping: 22,
      is_bot: false,
      is_verified: true,
      excellents: 2,
      impressives: 1,
      player_id: 201,
      model: 'razor',
    },
    {
      client_num: 1,
      name: 'DaHaNg',
      clean_name: 'DaHaNg',
      score: 3,
      ping: 34,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      excellents: 1,
      player_id: 202,
      model: 'keel',
    },
    // Spectator queue — order matters: first in this list is next to
    // play the winner when the current match ends.
    {
      client_num: 2,
      name: 'czm',
      clean_name: 'czm',
      score: 0,
      ping: 28,
      is_bot: false,
      is_verified: true,
      team: 3,
      player_id: 203,
    },
    {
      client_num: 3,
      name: 'Raisy',
      clean_name: 'Raisy',
      score: 0,
      ping: 41,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      team: 3,
      player_id: 204,
    },
    {
      client_num: 4,
      name: 'cha0ticz',
      clean_name: 'cha0ticz',
      score: 0,
      ping: 56,
      is_bot: false,
      // unverified — the rare variety amid the otherwise-verified roster.
      team: 3,
      player_id: 205,
    },
  ],
  server_vars: {
    fraglimit: '0',
    timelimit: '10',
    sv_hostname: 'Trinity 1v1',
    g_gametype: '1',
    g_gameplay: '0',
    g_movement: '0',
    mapname: 'q3tourney2',
  },
}
