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
    term: "autoexec.cfg",
    definition:
      "Quake 3 config file read on every launch and never overwritten by the engine. Where Trinity feature toggles, network tuning, and bindings persist. Lives in the baseq3 folder.",
  },
  {
    term: "baseq3",
    definition:
      "Quake 3's base game asset folder. Holds pak0.pk3 (the retail asset bundle), the 1.32 patch pk3s, and Trinity's autoexec.cfg / q3config.cfg.",
  },
  {
    term: "cvar",
    definition:
      "Quake 3's \"config variable\" — a runtime setting. Persistent cvars (CVAR_ARCHIVE) save to q3config.cfg on shutdown. Latched cvars (CVAR_LATCH) require a map_restart before changes take effect. Read-only cvars (CVAR_ROM) are managed by the engine and can't be set by the player.",
  },
  {
    term: "color codes",
    definition:
      "Three places in Quake 3 use single-digit color codes that work independently and disagree about which digit means which color: the player-settings color slider, the color1/color2 cvars (and the cg_enemyColors/cg_teamColors overrides built on them), and the ^N chat / player-name escapes. See Reference · Color codes for the side-by-side translation.",
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
    term: "pk3",
    definition:
      "Quake 3's PK3 archive format — a renamed ZIP file. Maps, models, sounds, and shaders ship in pk3 files placed in baseq3 (or missionpack). The engine loads the base game's pak0–pakN first, then any other pk3s in the folder in alphabetical order. When two custom pk3s contain the same asset, the later-loading one wins — so naming a conflicting pk3 later alphabetically (e.g. z-mymap.pk3) makes it the override.",
  },
  {
    term: "qkey",
    definition:
      "Quake 3's per-install identity file. Generated on first launch; the seed for your GUID. Lives in the engine's per-user data directory.",
  },
  {
    term: "RCON",
    definition:
      "Remote console. Quake 3's server admin protocol — issue commands to a remote q3 server using its rconpassword. Trinity's collector uses RCON to deliver welcome messages and !claim / !help replies back to players in-game.",
  },
  {
    term: "sv_dlURL",
    definition:
      'Server cvar for the base URL where clients fetch PK3s and TV demos via fastdl. Required by sv_tvDownload. Trinity\'s bundled bootstrap-nginx.sh stands up a matching dl.<your-host> vhost; the wizard sets this to "https://dl.<your-host>" automatically.',
  },
  {
    term: "Trinity Handshake",
    definition:
      "Set g_trinityHandshake 1 to gate a server to verified Trinity clients. With it on, non-Trinity clients are disconnected after 10 seconds, and logged-in Trinity players have their account auto-linked during the handshake — no !claim or !link needed. Without it, the hub rejects match stats from your server.",
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
