// Glossary terms surfaced in the Reference page. Sorted alphabetically
// with non-letter prefixes (! and digits) coming first. Each entry's
// `term` is the canonical display form; `definition` is plain prose
// (no JSX) so the search filter can match against it.

export interface GlossaryEntry {
  term: string;
  definition: string;
}

export const GLOSSARY: GlossaryEntry[] = [
  {
    term: "!claim",
    definition:
      "Trinity chat command that starts account creation. Run it in-game and the server replies with a six-digit code; enter that code at trinity.run to create your account and link the current player slot to it.",
  },
  {
    term: "!link",
    definition:
      "Trinity chat command that merges a stranded pre-account identity into your account. Used after creating an account if you played anonymously first.",
  },
  {
    term: "1.32 patch",
    definition:
      "Quake 3's 1.32 point release patches — pak1.pk3 through pak8.pk3 plus the missionpack patches. Required for Trinity to launch correctly.",
  },
  {
    term: "cvar",
    definition:
      "Latched cvars take effect on whichever restart re-reads them: vid_restart for renderer cvars, the next map for game cvars. Read-only cvars (CVAR_ROM) are managed by the engine and can't be set by the player.",
  },
  {
    term: "color codes",
    definition:
      "Three places in Quake 3 use single-digit color codes that work independently and disagree about which digit means which color: the player-settings color slider, the color1/color2 cvars (and the cg_enemyColors/cg_teamColors overrides built on them), and the ^N chat / player-name escapes. See Customize · Color codes for the side-by-side translation.",
  },
  {
    term: "Console tap",
    definition:
      "A live stream of the server console (sv_conTap 1) that powers trinity console <server>: remote rcon and console following for the server's operator and delegated admins, with no shell access to the game box.",
  },
  {
    term: "fastdl",
    definition:
      "HTTP-based fast download. A server-side nginx vhost on dl.<your-host> (provisioned by Trinity's bundled bootstrap-nginx.sh, served over HTTPS via Let's Encrypt) that delivers PK3s and TV demos so clients don't have to UDP-download them at the engine's slow built-in rate. Configured via sv_dlURL.",
  },
  {
    term: "GUID",
    definition:
      "Quake 3's per-client identifier, derived from your qkey. Trinity uses it to associate match stats with players. With cl_guidServerUniq 0, your GUID stays consistent across servers; 1 generates a different GUID per server.",
  },
  {
    term: "missionpack",
    definition:
      "Quake 3 Team Arena's asset folder. Required only if you want the Team Arena gametypes — One Flag CTF, Overload, Harvester. Trinity doesn't require it for vanilla CTF or other base gametypes.",
  },
  {
    term: "pak0",
    definition:
      "pak0.pk3 — the retail Quake 3 base assets archive. Comes in two copies: one in baseq3 (required for Trinity to run at all) and one in missionpack (required only for Team Arena gametypes like One Flag CTF, Overload, and Harvester). Both come from a legitimate Quake 3 install (Steam, retail CD, etc.).",
  },
  {
    term: "qkey",
    definition:
      "Quake 3's per-install identity file. Generated on first launch; the seed for your GUID. Lives in the engine's per-user data directory.",
  },
  {
    term: "RCON",
    definition:
      "Remote console (rconpassword). Trinity's collector uses it to deliver welcome messages and !claim / !help replies to players in-game.",
  },
  {
    term: "sv_dlURL",
    definition:
      "Server cvar for the fastdl base URL clients fetch PK3s and TV demos from. See Server Admin · Required server cvars.",
  },
  {
    term: "Trinity Handshake",
    definition:
      "g_trinityHandshake 1 gates a server to verified Trinity clients and links logged-in players' accounts during the handshake. See Server Admin · Required server cvars.",
  },
  {
    term: "TVD",
    definition:
      "TrinityVision Demo — Trinity's demo recording format. Servers record them automatically when sv_tvAuto is set, or on demand with the tvrecord / tvstop console commands; servers offer them to clients at end of match via sv_tvDownload + sv_dlURL. Playback works in the hub's web demo player and in any Trinity engine.",
  },
  {
    term: "Vadrigar",
    definition:
      'The unseen masters of the Arena Eternal in Quake III lore: they built the arenas and gather warriors to fight for their amusement. In Trinity, web viewers take their seat — when someone watches a live match from the site, players in the server hear "The Vadrigar watch." and an eye with the current viewer count appears on their HUD.',
  },
];
