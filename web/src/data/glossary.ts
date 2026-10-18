// Sorted alphabetically, with ! and digits first. Definitions are plain
// text so the search filter can match them.

export interface GlossaryEntry {
  term: string;
  definition: string;
}

export const GLOSSARY: GlossaryEntry[] = [
  {
    term: "!claim",
    definition:
      "Trinity chat command that starts account creation. Type it in-game and the server replies with a six-digit code; enter that code at trinity.run to create your account and link your current player to it.",
  },
  {
    term: "!link",
    definition:
      "Trinity chat command that links the install you're playing on to your account, using a code from your Account page. Use it for an install that didn't link when you logged in, or one you played on before you had an account.",
  },
  {
    term: "1.32 patch",
    definition:
      "Quake III's 1.32 point release: pak1.pk3 through pak8.pk3, plus the Team Arena patch files. Trinity needs them to launch, and the Trinity Installer fetches them if your install lacks them.",
  },
  {
    term: "cvar",
    definition:
      "A console variable: a named setting you can change in the console or in autoexec.cfg. Latched cvars take effect after the restart that rereads them: vid_restart for renderer cvars, the next map for game cvars. Read-only cvars are set by the engine.",
  },
  {
    term: "color codes",
    definition:
      "Three things in Quake III use single-digit color codes, and they disagree about which digit means which color: the color slider in the player-settings menu, the color1 and color2 cvars (and the cg_enemyColors and cg_teamColors strings built on them), and the ^N escapes in chat and player names. Color codes in Customize has the side-by-side translation.",
  },
  {
    term: "Console tap",
    definition:
      "A live stream of the server console (sv_conTap 1) behind trinity console <server>: remote rcon and console following for the server's operator and delegated admins, without shell access to the game box.",
  },
  {
    term: "fastdl",
    definition:
      "HTTP fast download: an nginx vhost on dl.<your-host>, served over HTTPS, that delivers pk3s and TV demos much faster than the engine's built-in UDP downloads. Set with sv_dlURL.",
  },
  {
    term: "GUID",
    definition:
      "Quake III's per-client identifier, made from your qkey. Trinity uses it to tie match stats to players. With cl_guidServerUniq 0, the default, your GUID is the same on every server; 1 gives you a different GUID per server.",
  },
  {
    term: "missionpack",
    definition:
      "Team Arena's asset folder, needed only for the Team Arena game types: One Flag CTF, Overload, and Harvester. Standard CTF and the other baseq3 game types work without it.",
  },
  {
    term: "pak0",
    definition:
      "pak0.pk3, the retail Quake III asset archive. There are two: one in baseq3, which Trinity always needs, and one in missionpack, needed only for Team Arena game types. Both come from your own copy of the game, from Steam or a retail CD.",
  },
  {
    term: "qkey",
    definition:
      "Quake III's per-install identity file, generated on first launch and used to make your GUID. It lives in the engine's per-user data folder.",
  },
  {
    term: "RCON",
    definition:
      "Remote console (rconpassword). Trinity's collector uses it to send welcome messages and !claim and !help replies to players in-game.",
  },
  {
    term: "sv_dlURL",
    definition:
      "Server cvar for the fast download base URL that clients fetch pk3s and TV demos from. See Required server cvars in Server admin.",
  },
  {
    term: "Trinity handshake",
    definition:
      "g_trinityHandshake 1 admits only verified Trinity clients and links logged-in players' accounts as they connect. See Required server cvars in Server admin.",
  },
  {
    term: "TVD",
    definition:
      "TrinityVision Demo, Trinity's demo recording format. Servers record them automatically when sv_tvAuto is set, or on demand with the tvrecord and tvstop console commands, and offer them to clients when a match ends through sv_tvDownload and sv_dlURL. They play back in the web demo player on this site and in any Trinity client.",
  },
  {
    term: "Vadrigar",
    definition:
      'The unseen masters of the Arena Eternal in Quake III lore: they built the arenas and gather warriors to fight for their amusement. In Trinity, web viewers take their seat: when someone watches a live match from the site, players in the server hear "The Vadrigar watch." and an eye with the current viewer count appears on their HUD.',
  },
];
