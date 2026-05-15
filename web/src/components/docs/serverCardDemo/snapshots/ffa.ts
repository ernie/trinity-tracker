// Canonical FFA snapshot for the Reading-a-server-card docs section.
//
// Captured from /api/servers/1/status then hand-edited so every
// per-variant tooltip surface in the FFA card is hoverable. PlayerBadge
// stays per-variant in the help text (six distinct icon/style
// combinations don't collapse cleanly to a single taxonomy string), so
// the roster includes one human per variant. BotBadge's HelpMode text
// IS a single taxonomy string, so we only need one bot — Anarki, whose
// `^1A^2n^3a^4r^5k^6i` funname doubles as the ColoredText demo.
//
// Variants demonstrated:
//   BotBadge: one bot (Anarki, skill 4 / Hardcore); HelpMode text
//     enumerates all five tiers regardless of which bot is hovered.
//   PlayerBadge variants:
//     ?  unverified         — evil
//     ?  unverified VR      — cypher
//     ✓  verified user      — av3k
//     ★  verified admin     — Cooller
//     ✓  verified user VR   — fox
//     ★  verified admin VR  — rapha
//   ColoredText rainbow: Anarki's funname
//   MedalIcon (FFA-relevant types): excellent, impressive, humiliation
//   SpectatorStrip: omitted on this card (varies across the docs section)
//
// Editorial rules locked in by the spec:
//   - Bot-list names ⇒ is_bot: true, ALWAYS.
//   - Humans showing PlayerBadge ⇒ non-bot handles (cooller, av3k, fox,
//     evil, cypher, rapha, daler — real-world Q3/QL competitive scene
//     names, none on trinity-bots.txt).
//   - server_id negative so LiveDataContext never matches the fixture.
//
// Raw capture is at ./ffa-raw.json (one-time artifact, not imported).
import type { ServerStatus } from '../../../../types'

export const FFA_SNAPSHOT: ServerStatus = {
  server_id: -1,
  source: 'hub',
  key: 'ffa',
  address: 'trinity.run:27960',
  map: 'q3dm1',
  game_type: 'ffa',
  game_time_ms: 487000, // ~8:07 elapsed
  max_clients: 12,
  human_count: 6,
  bot_count: 1,
  online: true,
  last_updated: '2026-05-14T22:17:45.612Z',
  match_state: 'active',
  // Sorted by score descending (matches the live card's render order).
  players: [
    {
      client_num: 0,
      name: 'Cooller',
      clean_name: 'Cooller',
      score: 14,
      ping: 24,
      is_bot: false,
      is_verified: true,
      is_admin: true,
      excellents: 4,
      impressives: 2,
      humiliations: 1,
      player_id: 101,
      model: 'sarge',
    },
    {
      client_num: 1,
      name: '^1A^2n^3a^4r^5k^6i',
      clean_name: 'Anarki',
      score: 12,
      ping: 0,
      is_bot: true,
      skill: 4,
      excellents: 3,
      impressives: 1,
      model: 'anarki',
    },
    {
      client_num: 2,
      name: '^1Nil^4Class',
      clean_name: 'NilClass',
      score: 10,
      ping: 31,
      is_bot: false,
      is_verified: true,
      is_admin: true,
      is_vr: true,
      excellents: 2,
      impressives: 1,
      player_id: 102,
      model: 'doom',
    },
    {
      client_num: 3,
      name: 'av3k',
      clean_name: 'av3k',
      score: 9,
      ping: 38,
      is_bot: false,
      is_verified: true,
      excellents: 2,
      player_id: 103,
      model: 'visor/blue',
    },
    {
      client_num: 4,
      name: 'Fox',
      clean_name: 'Fox',
      score: 7,
      ping: 52,
      is_bot: false,
      is_verified: true,
      is_vr: true,
      impressives: 1,
      player_id: 104,
      model: 'major',
    },
    {
      client_num: 5,
      name: 'Evil',
      clean_name: 'Evil',
      score: 5,
      ping: 46,
      is_bot: false,
      // unverified — no is_verified, no is_vr; PlayerBadge renders the
      // question-mark variant.
      humiliations: 1,
      player_id: 105,
      model: 'razor',
    },
    {
      client_num: 6,
      name: 'Cypher',
      clean_name: 'Cypher',
      score: 3,
      ping: 64,
      is_bot: false,
      is_vr: true,
      // unverified VR — is_vr set, is_verified not; PlayerBadge renders
      // the dotted-outline question-mark variant.
      player_id: 106,
      model: 'bones',
    },
    // No spectators on this card — variety across the docs section
    // ensures readers see that the SpectatorStrip can be absent.
  ],
  server_vars: {
    fraglimit: '20',
    timelimit: '15',
    sv_hostname: 'Trinity FFA',
    g_gametype: '0',
    g_gameplay: '0',
    g_movement: '0',
    mapname: 'q3dm1',
  },
}
