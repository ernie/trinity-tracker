import type { Platform } from "../components/docs/platformStorage";

// Trinity-introduced cvars across the three engines. Inventory built by
// walking Cvar_Get / CG_CVAR registrations in:
//   - flatscreen: ../trinity-engine + ../trinity (mod)
//   - pcvr:       ../trinity-vr (engine + bundled cgame)
//   - quest:      ../trinity-quest (engine + bundled cgame)
//
// Each entry is verified against the registering source. Defaults,
// platforms, and behavior reflect what the code actually does — no
// invented cvars, no hallucinated defaults. ROM/internal cvars
// (cl_trinityLoginStatus, cg_tvTime, cg_tvDuration, cg_tvdOffer) are
// excluded since users can't set them.
//
// Mostly Trinity additions, with a small handful of Quake3e-inherited
// renderer cvars (r_greyscale, r_mapGreyScale) included because they
// are genuinely player-facing visibility aids and surface in the
// player's guide alongside Trinity-introduced toggles.

export interface CvarValueDoc {
  value: string;
  meaning: string;
}

export interface CvarEntry {
  name: string;
  default: string;
  platforms: Platform[];
  description: string;
  // Optional value enumeration for cvars with discrete meaningful values.
  values?: CvarValueDoc[];
  // Optional human-readable note (e.g. "latched — requires map_restart",
  // "set by login flow — don't edit by hand").
  notes?: string;
}

const ALL_PLATFORMS: Platform[] = ["flatscreen", "pcvr", "quest"];
const VR_PLATFORMS: Platform[] = ["pcvr", "quest"];

// Player-facing cvars: cgame, ui, and Trinity-specific client engine
// cvars. Sorted alphabetically by name (Reference convention).
export const PLAYER_CVARS: CvarEntry[] = [
  {
    name: "cg_bloodParticles",
    default: "0",
    platforms: ALL_PLATFORMS,
    description: "Particle blood with wall and floor splats.",
  },
  {
    name: "cg_damageEffect",
    default: "0",
    platforms: ALL_PLATFORMS,
    description: "Directional red vignette overlay when taking damage.",
  },
  {
    name: "cg_damagePlums",
    default: "0",
    platforms: ALL_PLATFORMS,
    description: "Floating damage numbers above each hit.",
  },
  {
    name: "cg_deadBodyDarken",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Grey out dead bodies so they read as corpses. Requires pm or fb skins (the same skin family used by cg_enemyModel / cg_teamModel).",
  },
  {
    name: "cg_drawVoipSpeakers",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "List of currently-speaking players in the upper-right of the HUD, with channel-colored speaker icons.",
  },
  {
    name: "cg_enemyColors",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      "Up to five-digit color string for forced enemy models: head, body, legs, color1 (rail core / weapon glow), color2 (rail spiral). Shorter strings stop early — head/body/legs default to white if omitted; color1/color2 inherit from the enemy's own settings if omitted. Uses the same digit→color mapping as the player-settings color slider. See Reference · Color codes for the full mapping picture.",
    values: [
      { value: "1", meaning: "blue" },
      { value: "2", meaning: "green" },
      { value: "3", meaning: "cyan" },
      { value: "4", meaning: "red" },
      { value: "5", meaning: "magenta" },
      { value: "6", meaning: "yellow" },
      { value: "7", meaning: "white" },
      {
        value: "?",
        meaning: "becomes the player's team color (red, blue, or white in FFA)",
      },
    ],
  },
  {
    name: "cg_enemyModel",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      'Force every enemy to render as this model + skin pair (e.g. "keel/pm"). Empty disables. Skins suffixed with /pm (promode) or /fb (fullbright) accept the cg_enemyColors override.',
  },
  {
    name: "cg_followMode",
    default: "0",
    platforms: ["flatscreen"],
    description: "Spectator camera mode while following a player.",
    values: [
      { value: "0", meaning: "first-person follow" },
      { value: "1", meaning: "third-person orbit camera" },
      { value: "2", meaning: "free-fly (TV playback only)" },
    ],
  },
  {
    name: "cg_hitSounds",
    default: "0",
    platforms: ALL_PLATFORMS,
    description: "Damage-scaled hit sound feedback.",
    values: [
      { value: "0", meaning: "off" },
      { value: "1", meaning: "lower pitch on bigger hits" },
      { value: "2", meaning: "higher pitch on bigger hits" },
    ],
  },
  {
    name: "cg_smoothFollow",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "Orbit camera for VR third-person spectating. Default 0: the third-person camera snaps to a new position when context changes (player switch, recenter). 1: a continuous orbit you steer with the thumbstick — rotate around the player, zoom in/out, B to recenter. Lives in the in-game Comfort menu because continuous camera movement in VR takes strong VR legs.",
  },
  {
    name: "cg_teamColors",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      "Up to five-digit color string for forced teammate models: head, body, legs, color1, color2. See cg_enemyColors for the value list and the bail-early behavior.",
  },
  {
    name: "cg_teamModel",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      'Force every teammate to render as this model + skin pair (e.g. "doom/pm"). Empty disables.',
  },
  {
    name: "cg_trinityAnnounce",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Play audio announcements for players when they join a server and when they win a match in FFA or 1v1. Eligible players have a custom announcer clip in the zzz-trinity-announcer.pk3 that ships alongside the mod.",
    values: [
      { value: "0", meaning: "off" },
      { value: "1", meaning: "on" },
    ],
  },
  {
    name: "cg_trueShotgun",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Show the fixed Quake Live ring pellet pattern instead of cosmetic random spread. Only takes effect when g_gameplay is set to Quake Live (other gameplay profiles use their own spread patterns).",
  },
  {
    name: "cg_tvSkip",
    default: "10",
    platforms: ALL_PLATFORMS,
    description:
      "Number of seconds to skip when scrubbing forward or backward during TV demo playback.",
  },
  {
    name: "cg_tvTimeline",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Show the timeline scrubber UI when watching TV demos. 0 hides it.",
  },
  {
    name: "cg_tvdTimeout",
    default: "10",
    platforms: ALL_PLATFORMS,
    description:
      "Seconds the 'Download last match?' prompt stays up before timing out. The prompt appears at the start of the next match if the server offered a TVD download (sv_tvDownload + sv_dlURL).",
  },
  {
    name: "cl_trinityToken",
    default: "",
    platforms: ALL_PLATFORMS,
    description: "Trinity authentication token.",
    notes:
      "Normally set by the in-game Account login flow. Power users can paste a long-lived game token from their Account page into autoexec.cfg directly to pre-authenticate a fresh install — see Docs · Account. Marked CVAR_PROTECTED so it can't be exported via /cvarlist.",
  },
  {
    name: "cl_trinityTracker",
    default: "https://trinity.run",
    platforms: ALL_PLATFORMS,
    description:
      "Base URL for the Trinity Tracker hub. Override only if you run your own hub.",
  },
  {
    name: "cl_trinityUser",
    default: "",
    platforms: ALL_PLATFORMS,
    description: "Logged-in Trinity username.",
    notes:
      "Normally set by the in-game Account login flow. Power users can pre-set this in autoexec.cfg paired with cl_trinityToken — see Docs · Account.",
  },
  {
    name: "cl_tvDownload",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "TV demo download offer behavior at end of match. The prompt appears at the start of the next match.",
    values: [
      { value: "0", meaning: "off (no prompt)" },
      { value: "1", meaning: "prompt; defaults to decline if no response" },
      { value: "2", meaning: "prompt; defaults to accept if no response" },
    ],
  },
  {
    name: "r_greyscale",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Desaturate the entire rendered frame, entities included. Range -1 to 1; 1 is full greyscale, 0 is full color. Requires r_fbo 1. Inherited from Quake3e — useful as a screenshot / aesthetic option, or paired with r_mapGreyScale.",
  },
  {
    name: "r_mapGreyScale",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Desaturate world map textures only — entities (players, weapons, items, projectiles, explosions) stay in full color. Practical visibility aid: enemies and powerups pop against grey walls. Range -1 to 1; positive values desaturate textures and lightmaps, negative values desaturate only lightmaps. Inherited from Quake3e.",
    notes:
      "Latched — requires vid_restart (or set in autoexec.cfg before launch) to take effect.",
  },
  {
    name: "ui_trinitySigil",
    default: "1",
    platforms: ALL_PLATFORMS,
    description: "3D Trinity sigil in the main menus. 0 hides it.",
  },
];

// VR comfort, control, and rendering cvars. Curated set of the most-
// tweaked values; the full vr_button_map_* and vr_weapon_adjustment_*
// inventories live in the starter autoexec configs (typically tuned
// there, not at runtime). PCVR + Quest unless noted.
export const VR_CVARS: CvarEntry[] = [
  {
    name: "vr_6dof",
    default: "1",
    platforms: VR_PLATFORMS,
    description:
      "True 6DoF body tracking. 1 drives the in-game player position directly from real-world movement — single-player only. 0 translates body motion into Quake 3 movement input commands, which is what the multiplayer protocol expects.",
  },
  {
    name: "vr_comfortVignette",
    default: "0.0",
    platforms: VR_PLATFORMS,
    description:
      "Comfort vignette (tunnel-vision darkening) intensity during motion. Higher values reduce VR motion sickness at the cost of peripheral visibility.",
  },
  {
    name: "vr_directionMode",
    default: "1",
    platforms: VR_PLATFORMS,
    description: "Movement direction reference when moving.",
    values: [
      { value: "0", meaning: "follow head (HMD) orientation" },
      { value: "1", meaning: "follow off-hand controller orientation" },
    ],
  },
  {
    name: "vr_hapticIntensity",
    default: "0.5",
    platforms: VR_PLATFORMS,
    description:
      "Controller vibration / haptic feedback intensity, 0.0 to 1.0.",
  },
  {
    name: "vr_hudDepth",
    default: "3",
    platforms: VR_PLATFORMS,
    description:
      "How far the HUD renders in front of the player in VR space (range 0–5; higher = farther away).",
  },
  {
    name: "vr_hudScale",
    default: "1.0",
    platforms: VR_PLATFORMS,
    description: "HUD size multiplier in VR.",
  },
  {
    name: "vr_hudYOffset",
    default: "0",
    platforms: VR_PLATFORMS,
    description: "Vertical HUD position offset.",
  },
  {
    name: "vr_lasersight",
    default: "0",
    platforms: VR_PLATFORMS,
    description: "Show a laser sight from the muzzle of held weapons.",
  },
  {
    name: "vr_refreshrate",
    default: "90",
    platforms: ["quest"],
    description:
      "Quest headset display refresh rate (typical: 72, 90, 120). Pair with com_maxfps 0 so the engine tracks the headset.",
  },
  {
    name: "vr_righthanded",
    default: "1",
    platforms: VR_PLATFORMS,
    description: "Handedness. 1 = right-handed, 0 = left-handed.",
  },
  {
    name: "vr_rollWhenHit",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "Roll the HMD view when taking damage. Off by default — uncomfortable for many players.",
  },
  {
    name: "vr_screenCurvature",
    default: "0.5",
    platforms: ["quest"],
    description:
      "Curvature of the virtual screen used for flat-screen content (menus, console). 0 = flat, 1 = max curve.",
  },
  {
    name: "vr_snapturn",
    default: "45",
    platforms: VR_PLATFORMS,
    description: "Snap turn angle in degrees per stick flick.",
    values: [
      { value: "0", meaning: "smooth turn (continuous rotation)" },
      { value: "N", meaning: "snap by N degrees per flick (e.g. 45)" },
    ],
  },
  {
    name: "vr_superSampling",
    default: "1.0",
    platforms: ["quest"],
    description: "Render supersampling factor (1.0 = native, >1.0 = upsample).",
    notes: "Latched — requires engine restart to take effect.",
  },
  {
    name: "vr_switchThumbsticks",
    default: "0",
    platforms: VR_PLATFORMS,
    description: "Swap primary / secondary thumbstick assignments.",
  },
  {
    name: "vr_thumbstickDeadzone",
    default: "0.15",
    platforms: VR_PLATFORMS,
    description:
      "Thumbstick dead-zone threshold (input below this is treated as zero).",
  },
  {
    name: "vr_thumbstickFullDeflection",
    default: "0.85",
    platforms: VR_PLATFORMS,
    description:
      "Thumbstick full-deflection threshold (input above this is treated as max).",
  },
  {
    name: "vr_triggerSensitivity",
    default: "0.25",
    platforms: VR_PLATFORMS,
    description: "Trigger pull threshold for fire (range 0.1–0.9).",
  },
  {
    name: "vr_twoHandedWeapons",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "Two-handed weapon stabilization mode — controls how the weapon points when you hold the secondary grip with your off hand.",
    values: [
      { value: "0", meaning: "off (primary hand only)" },
      {
        value: "1",
        meaning:
          "basic — weapon aims along a line from your weapon hand toward your off-hand position",
      },
      {
        value: "2",
        meaning:
          "virtual gun stock — weapon anchors near your face, like sighting along a stock",
      },
    ],
  },
  {
    name: "vr_uturn",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "Enable a 180° U-turn snap (typically bound to a controller button).",
  },
  {
    name: "vr_weaponScope",
    default: "1",
    platforms: VR_PLATFORMS,
    description: "Enable railgun zoom in VR.",
  },
  {
    name: "vr_worldscale",
    default: "32.0",
    platforms: VR_PLATFORMS,
    description:
      "Scale of the in-game world relative to your real-world body. Default 32 = 1 Quake unit per 1/32 meter, the engine's classic ratio. Lower values shrink the world (you feel taller); higher values enlarge it.",
  },
];

// Server-side cvars (game module + engine server). Server builds are
// flatscreen (trinity-engine ded), so platform tagging isn't shown
// per-cvar in the Reference UI for this section — these apply when
// you run a Trinity server.
export const SERVER_CVARS: CvarEntry[] = [
  {
    name: "g_gameplay",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Gameplay rules profile — affects per-weapon damage and knockback, ammo pickup amounts, armor mechanics (CPMA adds a tiered system with a lower-tier green armor on top of yellow and red), spawn health, and item respawn timing.",
    values: [
      { value: "0", meaning: "Quake 3" },
      { value: "1", meaning: "CPMA" },
      { value: "2", meaning: "Quake Live" },
    ],
    notes: "Latched — requires map_restart to take effect.",
  },
  {
    name: "g_logSync",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Flush the game log to disk synchronously after each event. The Trinity collector tails events in real time, so this is required for collector participation.",
  },
  {
    name: "g_movement",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Movement physics profile (acceleration, air control, friction).",
    values: [
      { value: "0", meaning: "Quake 3" },
      { value: "1", meaning: "CPMA" },
      { value: "2", meaning: "Quake Live" },
      { value: "3", meaning: "Quake Live Turbo" },
    ],
    notes: "Latched — requires map_restart to take effect.",
  },
  {
    name: "g_overtimelimit",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Cap sudden-death overtime in minutes. 0 is unbounded — a tied match can produce huge demo files (and matches that never end). Pick a value that fits your mode; team modes like CTF often want more than duels.",
  },
  {
    name: "g_rotation",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Path to a map rotation file for automated map cycling. Empty / 0 disables rotation.",
  },
  {
    name: "g_teamDMSpawnThreshold",
    default: "8",
    platforms: ALL_PLATFORMS,
    description:
      "Use team / CTF spawn points on Team Deathmatch maps that have fewer FFA spawns than this value. Avoids telefrag-fests on small maps.",
  },
  {
    name: "g_trinityHandshake",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Gate the server to verified Trinity clients. Without it, the hub rejects match stats and hides the server from dashboard cards and the activity feed. With it, non-Trinity clients are disconnected after 10 seconds, and logged-in Trinity players have their account credentials verified automatically during the handshake — their GUID is linked to their account with no !claim or !link needed.",
    notes:
      "Required (= 1) for the server to participate in the Trinity network.",
  },
  {
    name: "sv_dlRate",
    default: "100",
    platforms: ALL_PLATFORMS,
    description:
      "Bandwidth allotted to PK3 file downloads via UDP, in kilobytes per second. Separate from HTTP / fastdl downloads (those go through sv_dlURL).",
  },
  {
    name: "sv_dlURL",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      'Base URL for asset downloads (PK3s and TV demos). Required by sv_tvDownload. The bundled scripts/bootstrap-nginx.sh stands up a matching dl.<your-host> vhost with a Let\'s Encrypt cert; the wizard sets this to "https://dl.<your-host>" automatically when it has a public URL.',
  },
  {
    name: "sv_tvAuto",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Auto-record TV demos on every map load. Required for collector participation: matches played on your server need to be watchable through the hub.",
  },
  {
    name: "sv_tvAutoMinPlayers",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Minimum concurrent non-spectator human players to keep an auto-recording. 0 = always keep, even bot-only matches. Pair with sv_tvAutoMinPlayersSecs.",
  },
  {
    name: "sv_tvAutoMinPlayersSecs",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Seconds the player threshold must be continuously met before discarding a recording. 0 = instantaneous. Pair with sv_tvAutoMinPlayers.",
  },
  {
    name: "sv_tvDownload",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Notify clients to download TV recordings via HTTP at end of match. Requires sv_dlURL.",
  },
  {
    name: "sv_tvpath",
    default: "demos",
    platforms: ALL_PLATFORMS,
    description:
      "Directory for TV recordings, relative to the active game asset folder — baseq3 (or missionpack on Team Arena gametypes), not the Trinity install root. The default 'demos' writes .tvd files alongside regular client-side demo recordings.",
  },
];
