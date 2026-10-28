import type { Platform } from "../components/docs/platformStorage";

// Trinity-added cvars, plus the Quake3e renderer cvars the Play page
// covers (r_greyscale, r_mapGreyScale). Defaults come from the
// registering code in ../trinity-engine and ../trinity. Internal ROM cvars
// (cl_trinityLoginStatus, cg_tvTime, cg_tvDuration, cg_tvdOffer) stay out;
// read-only cvars a player may check (cl_platform, r_hdrActive) are listed
// and marked read-only.

export interface CvarValueDoc {
  value: string;
  meaning: string;
}

export interface CvarEntry {
  name: string;
  default: string;
  // Platforms whose engine registers a different default than `default`.
  // The reference resolves this against the reader's active platform so
  // nobody is shown another platform's value.
  defaultByPlatform?: Partial<Record<Platform, string>>;
  platforms: Platform[];
  description: string;
  // Optional value enumeration for cvars with discrete meaningful values.
  values?: CvarValueDoc[];
  // Optional human-readable note (e.g. "Takes effect after a map_restart.").
  notes?: string;
}

const ALL_PLATFORMS: Platform[] = ["flatscreen", "vr"];
const VR_PLATFORMS: Platform[] = ["vr"];
// HDR display output reaches a monitor: flat play, plus the desktop
// mirror of VR on a PC.
const HDR_PLATFORMS: Platform[] = ["flatscreen", "vr"];

// Player-facing cvars: cgame, ui, and Trinity-specific client engine
// cvars. Sorted alphabetically by name (Reference convention).
export const PLAYER_CVARS: CvarEntry[] = [
  {
    name: "cg_bloodDensity",
    default: "2",
    platforms: ALL_PLATFORMS,
    description:
      "How much modern blood (com_blood 2) is drawn. Setup → Game Options → Blood Density.",
    values: [
      { value: "0", meaning: "Light" },
      { value: "1", meaning: "Medium" },
      { value: "2", meaning: "Heavy" },
    ],
    notes:
      "Defaults to 2 (Heavy) on a PC and 0 (Light) on Steam Frame, Quest, and PICO.",
  },
  {
    name: "cg_damageEffect",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "A red vignette on the side of the screen a hit came from when you take damage. 0 shows the classic blood blob. Setup → Game Options → Damage Effect.",
  },
  {
    name: "cg_damagePlums",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Floating damage numbers above each hit. Setup → Game Options → Damage Numbers.",
  },
  {
    name: "cg_damagePlumScale",
    default: "1.0",
    platforms: ALL_PLATFORMS,
    description: "Size multiplier for cg_damagePlums numbers.",
  },
  {
    name: "cg_deadBodyDarken",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Gray out dead bodies so they read as corpses. Requires pm or fb skins.",
  },
  {
    name: "cg_drawSpeed",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Speedometer reading your current movement speed in units per second.",
    values: [
      { value: "0", meaning: "off" },
      { value: "1", meaning: "upper right, stacked with the other readouts" },
      { value: "2", meaning: "centered low on the screen" },
    ],
  },
  {
    name: "cg_drawTimer",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Match clock in the HUD, counting down to the timelimit and prefixing overtime with OT. Setup → Game Options → Show Time.",
  },
  {
    name: "cg_drawViewers",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Show the Vadrigar eye and viewer count on your HUD while someone is watching the match live from the website.",
  },
  {
    name: "cg_drawVoipSpeakers",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "List of the players speaking right now in the upper right of the HUD, with channel-colored speaker icons.",
  },
  {
    name: "cg_enemyColors",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      "A color string of up to five digits for forced enemy models: head, body, legs, color1 (rail core and weapon glow), and color2 (rail spiral). Omitted head, body, or legs digits default to white; omitted color1 and color2 keep the enemy's own settings. The digits mean the same colors as the color1 cvar.",
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
      'Every enemy renders as this model and skin, such as "keel/pm". Skins ending in /pm (promode) or /fb (fullbright) accept cg_enemyColors.',
  },
  {
    name: "cg_followMode",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Which camera the follow view starts in while spectating or watching a TV demo, flatscreen or VR. Switching cameras in-game saves the choice here. In VR the turn stick orbits the third-person camera. Setup → Game Options → Follow Camera.",
    values: [
      { value: "0", meaning: "first-person follow" },
      { value: "1", meaning: "third-person orbit camera" },
      { value: "2", meaning: "free-fly (TV playback only)" },
    ],
  },
  {
    name: "cg_fovAdjust",
    default: "1",
    platforms: ["flatscreen"],
    description:
      "Widen cg_fov to suit the display's aspect ratio, so a wide monitor shows more at the sides rather than cropping top and bottom. A 4:3 display is unaffected.",
  },
  {
    name: "cg_fragMessage",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Center-screen confirmation naming who you just fragged. In free-for-all games it also reports your resulting place and score.",
  },
  {
    name: "cg_hitSounds",
    default: "0",
    platforms: ALL_PLATFORMS,
    description: "The sound a hit makes. Setup → Game Options → Hit Sounds.",
    values: [
      {
        value: "0",
        meaning:
          "Classic: the game's own, baseq3's single beep or Team Arena's armor tones",
      },
      { value: "1", meaning: "Big Hits Low: lower pitch on bigger hits" },
      { value: "2", meaning: "Big Hits High: higher pitch on bigger hits" },
    ],
  },
  {
    name: "cg_teamColors",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      "A color string of up to five digits for forced teammate models: head, body, legs, color1, and color2. See cg_enemyColors for the digits and what omitted ones do.",
  },
  {
    name: "cg_teamModel",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      'Every teammate renders as this model and skin, such as "doom/pm".',
  },
  {
    name: "cg_trinityAnnounce",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Play audio announcements for players when they join a server and when they win a match in FFA or 1v1. Eligible players have a custom announcer clip in the zzz-trinity-announcer.pk3 that ships alongside the mod.",
  },
  {
    name: "cg_trueShotgun",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Show the fixed Quake Live ring pellet pattern instead of cosmetic random spread. Only takes effect under a Quake Live mode (Quake Live or Quake Live Turbo); other modes use their own spread patterns.",
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
    description: "Show the timeline scrubber UI when watching TV demos.",
  },
  {
    name: "cg_tvdTimeout",
    default: "10",
    platforms: ALL_PLATFORMS,
    description:
      "Seconds the 'Download last match?' prompt stays up before timing out. The prompt appears at the start of the next match if the server offered a TVD download (sv_tvDownload + sv_dlURL).",
  },
  {
    name: "cg_weaponSelectorGauntlet",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "Put the gauntlet on the weapon wheel's default set, first in order. Setup → VR Options → Controls → Gauntlet On Weapon Wheel. A custom cg_weaponSelectorWeapons list ignores it.",
  },
  {
    name: "cg_weaponSelectorWeapons",
    default: "(empty)",
    platforms: VR_PLATFORMS,
    description:
      "The weapons on the weapon wheel, in order; the last lands in the top slot. Empty uses the default set: every weapon in weapon order, the gauntlet only with cg_weaponSelectorGauntlet, and the grapple last whenever the server hands it out. Tokens are weapon numbers or item names (with or without the weapon_ prefix); * inserts the default set, and !name hides a weapon from later tokens and * (put exclusions first). Not saved, so set it in autoexec.cfg.",
  },
  {
    name: "cl_packetdup",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Rides out that many packets lost in a row; each step up costs a little more upload. Setup → Network → Packet Recovery: Low (1), Medium (2), or High (3).",
  },
  {
    name: "cl_platform",
    default: "(your device)",
    platforms: ALL_PLATFORMS,
    description: "The device the client runs on.",
    values: [
      { value: "windows", meaning: "Windows PC" },
      { value: "macos", meaning: "Mac" },
      { value: "linux", meaning: "Linux PC" },
      { value: "frame", meaning: "Steam Frame" },
      { value: "pico", meaning: "PICO" },
      { value: "quest", meaning: "Meta Quest" },
    ],
    notes: "Read-only: it reports the platform, and you can't set it.",
  },
  {
    name: "cl_trinityToken",
    default: "",
    platforms: ALL_PLATFORMS,
    description: "Trinity authentication token.",
    notes:
      "Set when you log in from the in-game Account menu. You can also paste the game token from your Account page into autoexec.cfg to log in a fresh install; see the Account docs. Servers and mods can't change it.",
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
      "Set when you log in from the in-game Account menu. You can also set it in autoexec.cfg along with cl_trinityToken; see the Account docs.",
  },
  {
    name: "cl_tvDownload",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "The offer to download the match's TV demo. The prompt appears at the start of the next match. Setup → Game Options → TVD Download.",
    values: [
      { value: "0", meaning: "Off: no prompt" },
      { value: "1", meaning: "Decline: prompt, declining if you don't answer" },
      { value: "2", meaning: "Accept: prompt, accepting if you don't answer" },
    ],
  },
  {
    name: "com_blood",
    default: "2",
    platforms: ALL_PLATFORMS,
    description:
      "Modern blood reacts to how hard you hit: bigger hits throw more blood, gibs fly in the direction of the killing blow, and splatter pools on the walls and floor around the kill. Setup → Game Options → Blood. cg_bloodDensity sets how much modern blood is drawn.",
    values: [
      { value: "0", meaning: "off" },
      { value: "1", meaning: "Classic: the original Quake III sprite blood" },
      {
        value: "2",
        meaning:
          "Modern: blood, spray, trails, and marks that scale with damage",
      },
    ],
  },
  {
    name: "r_displayRefresh",
    default: "0",
    platforms: ["flatscreen"],
    description:
      "Refresh rate for exclusive fullscreen, in Hz. 0 uses the mode's own rate. Windowed and Borderless always run at the desktop's rate. Setup → System → Graphics → Refresh Rate.",
    notes: "Takes effect after a vid_restart.",
  },
  {
    name: "r_flares",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Corona glows on light sources: lamps, torches, and the sun bloom out when you look near them.",
  },
  {
    name: "r_flareSize",
    default: "40",
    platforms: ALL_PLATFORMS,
    description: "Radius of the corona drawn by r_flares. Range 1 to 40.",
  },
  {
    name: "r_fullscreen",
    default: "2",
    platforms: ["flatscreen"],
    description: "Window mode. Setup → System → Graphics → Window Mode.",
    values: [
      { value: "0", meaning: "Windowed" },
      {
        value: "1",
        meaning:
          "Fullscreen: exclusive, at the chosen resolution and refresh rate",
      },
      {
        value: "2",
        meaning:
          "Borderless: covers the desktop at its own resolution and refresh rate",
      },
    ],
    notes:
      "Takes effect after a vid_restart. Alt+Enter switches to windowed and back to whichever fullscreen kind you used last.",
  },
  {
    name: "r_greyscale",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Desaturate the entire rendered frame, entities included. Range -1 to 1; 1 is full grayscale, 0 is full color. Requires r_fbo 1. From Quake3e.",
  },
  {
    name: "r_hdrActive",
    default: "0",
    platforms: HDR_PLATFORMS,
    description:
      "1 while HDR output is on, which needs an HDR-capable display with HDR switched on in your OS; otherwise 0.",
    notes: "Read-only: it reports HDR state, and you can't set it.",
  },
  {
    name: "r_hdrDisplay",
    default: "0",
    platforms: HDR_PLATFORMS,
    description:
      "Output HDR, with brighter highlights. Requires the Vulkan renderer and an HDR-capable display with HDR turned on in your OS. In VR on a PC it applies to the desktop mirror window. r_hdr is a different setting, for internal rendering precision. Setup → Display → HDR Display.",
    notes: "Takes effect after a vid_restart.",
  },
  {
    name: "r_hdrHighlightBoost",
    default: "1.0",
    platforms: HDR_PLATFORMS,
    description:
      "How bright the brightest highlights get in HDR: lights, explosions, plasma and sky. 1 is natural; higher makes them brighter but makes very bright highlights look more alike, lower calms them. Does not change overall brightness. Range 0.5 to 2.0.",
    notes:
      "Setup → Display → Highlight Boost, available while HDR output is on.",
  },
  {
    name: "r_hdrPaperWhite",
    default: "0",
    platforms: HDR_PLATFORMS,
    description:
      "How bright normal white looks in HDR (menus, HUD, fully-lit surfaces), in nits. 0 picks a comfortable level from your display's peak; any other value sets it directly. Range 0 to 1000. On macOS the system sets the white level, so this has no effect there.",
  },
  {
    name: "r_hdrPeak",
    default: "400",
    platforms: HDR_PLATFORMS,
    description:
      "Your HDR display's usable peak brightness in nits, the main HDR setting. Highlights brighten up to this, and the automatic white level is based on it. Set it on the HDR Calibration screen (Setup → Display), which finds the brightness your panel really reaches, often well below its rated peak. The screen goes up to 2000 nits; set by hand, the cvar accepts 250 to 10000. On macOS the system reports the display's peak, so this has no effect there and the calibration screen isn't offered.",
    notes:
      "Flatscreen and VR share the value, so a peak calibrated in either applies to both.",
  },
  {
    name: "r_hdrSaturation",
    default: "0.0",
    platforms: HDR_PLATFORMS,
    description:
      "How far the brightest highlights bleed toward white in HDR. 1 keeps full color; 0 lets the brightest emitters go fully white. It mainly helps intense emitters, blues especially, read as bright. Range 0 to 1.",
  },
  {
    name: "r_hdrSaturationFull",
    default: "1.5",
    platforms: HDR_PLATFORMS,
    description:
      "Works with r_hdrSaturation: how intense a highlight must be before it reaches its maximum bleed toward white. Lower makes bright cores whiten sooner. Range 0.5 to 3.0.",
  },
  {
    name: "r_hdrSdrCurve",
    default: "1",
    platforms: HDR_PLATFORMS,
    description:
      "How HDR output decodes SDR content: the menus, the HUD and everything below paper white. Also the SDR Gamma row under Setup → Display.",
    values: [
      {
        value: "0",
        meaning:
          "sRGB, as Windows shows SDR apps in HDR; darks look lifted and colors a little washed out",
      },
      {
        value: "1",
        meaning: "2.2, matching how the game looks on an SDR monitor",
      },
    ],
  },
  {
    name: "r_hdrSoftKnee",
    default: "1.0",
    platforms: HDR_PLATFORMS,
    description:
      "How gradually the HDR highlight treatment eases in. 0 switches it in abruptly, which can show as a band of brightness; higher eases it in more gently. Range 0 to 2.",
  },
  {
    name: "r_mapGreyScale",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Desaturate the world map only; entities (players, weapons, items, projectiles, explosions) stay in full color, so enemies and powerups stand out against gray walls. Range -1 to 1; positive values desaturate textures and lightmaps, negative values only lightmaps. From Quake3e.",
    notes: "Takes effect after a vid_restart, or set it in autoexec.cfg.",
  },
  {
    name: "r_shadowClip",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Clip stencil shadows (cg_shadows 2) against level geometry, so they stop at walls and floors.",
  },
  {
    name: "r_shadowClipExtension",
    default: "16",
    platforms: ALL_PLATFORMS,
    description:
      "How far a clipped shadow may stretch back toward the surface it falls on, in game units. Raising it closes gaps where a shadow stops short; lowering it keeps shadows tighter to the caster.",
  },
  {
    name: "r_shadowClipPenetration",
    default: "4",
    platforms: ALL_PLATFORMS,
    description:
      "How far a shadow may pass through a surface before it's clipped, in game units. Smaller values clip sooner.",
  },
  {
    name: "r_shadowDistance",
    default: "256",
    platforms: ALL_PLATFORMS,
    description:
      "How far from the caster a stencil shadow reaches, in game units. Lower values save frame time.",
  },
];

// vr_weapon_adjustment_* stays out because in-game weapon adjust sets it.
export const VR_CVARS: CvarEntry[] = [
  {
    name: "vr_6dof",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "6DoF body tracking. 1 moves your in-game position directly with your real-world movement, in single-player only. 0 turns your movement into normal movement input, which multiplayer uses. Setup → VR Options → Comfort.",
  },
  {
    name: "vr_comfortVignette",
    default: "0.0",
    platforms: VR_PLATFORMS,
    description:
      "Comfort vignette (tunnel-vision darkening) intensity during motion.",
  },
  {
    name: "vr_directionMode",
    default: "1",
    platforms: VR_PLATFORMS,
    description:
      "What forward means when you move. Setup → VR Options → Controls → Direction Mode.",
    values: [
      { value: "0", meaning: "HMD: follows your head" },
      { value: "1", meaning: "Off-hand Controller: follows your off hand" },
    ],
  },
  {
    name: "vr_foveation",
    default: "2",
    platforms: VR_PLATFORMS,
    description:
      "Foveated rendering draws the edges of your view in less detail than the center, freeing up GPU time. Setup → VR Options → HUD & Display → Foveated Rendering. A mode your headset can't do falls back to one it can, and the cvar changes to match.",
    values: [
      { value: "0", meaning: "Off" },
      { value: "1", meaning: "Fixed: sharp region at the center of each eye" },
      {
        value: "2",
        meaning:
          "Eye-Tracked: sharp region follows your gaze; headsets with eye tracking only",
      },
    ],
  },
  {
    name: "vr_foveationStrength",
    default: "2",
    platforms: VR_PLATFORMS,
    description:
      "How much detail the edges give up under foveated rendering. Setup → VR Options → HUD & Display → Foveation Strength.",
    values: [
      { value: "1", meaning: "Low" },
      { value: "2", meaning: "Medium" },
      { value: "3", meaning: "High: most GPU time saved, most visible" },
    ],
  },
  {
    name: "vr_hapticIntensity",
    default: "0.5",
    platforms: VR_PLATFORMS,
    description:
      "Controller vibration strength, 0.0 to 1.0; higher is stronger.",
  },
  {
    name: "vr_hudDepth",
    default: "3",
    platforms: VR_PLATFORMS,
    description:
      "How far in front of you the HUD sits. Range 0 to 5; higher is farther away. Setup → VR Options → HUD & Display → HUD Depth.",
  },
  {
    name: "vr_hudScale",
    default: "1.0",
    platforms: VR_PLATFORMS,
    description:
      "HUD size multiplier, 0.5 to 2.0 in the menu; higher is larger. Setup → VR Options → HUD & Display → HUD Scale.",
  },
  {
    name: "vr_hudYOffset",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "Vertical HUD offset, -200 to 200 in the menu; positive raises the HUD.",
  },
  {
    name: "vr_lasersight",
    default: "0",
    platforms: VR_PLATFORMS,
    description: "Show a laser sight from the muzzle of held weapons.",
  },
  {
    name: "vr_physicalCrouch",
    default: "1",
    platforms: VR_PLATFORMS,
    description:
      "Crouch in game when you crouch in real life, alongside the Crouch button. It doesn't apply while swimming or flying. Setup → VR Options → Comfort → Physical Crouch.",
    values: [
      { value: "0", meaning: "Off" },
      { value: "1", meaning: "On" },
    ],
  },
  {
    name: "vr_refreshrate",
    default: "90",
    platforms: VR_PLATFORMS,
    description:
      "Headset display refresh rate, in Hz. Setup → VR Options → HUD & Display → Refresh Rate lists only the rates your headset's runtime reports, lowest first; a value it can't do snaps to the nearest one it can. Applies live.",
  },
  {
    name: "vr_righthanded",
    default: "1",
    platforms: VR_PLATFORMS,
    description: "Handedness: 1 is right-handed, 0 left-handed.",
  },
  {
    name: "vr_rollWhenHit",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "Roll the HMD view when taking damage. Uncomfortable for many players.",
  },
  {
    name: "vr_screenCurvature",
    default: "0.5",
    platforms: VR_PLATFORMS,
    description:
      "Curvature of the virtual screen that shows flatscreen content such as menus and the console. 0 is flat, 1 the most curved.",
  },
  {
    name: "vr_snapturn",
    default: "0",
    platforms: VR_PLATFORMS,
    description:
      "Snap turn angle in degrees per stick flick. Setup → VR Options → Controls → Turning Mode offers smooth, 45, and 90.",
    values: [
      { value: "0", meaning: "smooth turning" },
      { value: "1", meaning: "snap by 45 degrees per flick" },
      { value: "N", meaning: "snap by N degrees per flick, such as 30" },
    ],
  },
  {
    name: "vr_superSampling",
    default: "1.0",
    platforms: VR_PLATFORMS,
    description:
      "Supersampling factor: 1.0 renders at the headset's native resolution, and higher renders more pixels.",
    notes: "Takes effect after restarting Trinity.",
  },
  {
    name: "vr_switchThumbsticks",
    default: "0",
    platforms: VR_PLATFORMS,
    description: "1 swaps the move and turn sticks between hands.",
  },
  {
    name: "vr_thumbstickDeadzone",
    default: "0.15",
    platforms: VR_PLATFORMS,
    description: "How far you push a thumbstick before it registers.",
  },
  {
    name: "vr_thumbstickFullDeflection",
    default: "0.85",
    platforms: VR_PLATFORMS,
    description: "How far you push a thumbstick to reach full speed.",
  },
  {
    name: "vr_triggerSensitivity",
    default: "0.25",
    platforms: VR_PLATFORMS,
    description:
      "How light a trigger pull fires the weapon. Higher is more sensitive, so a shorter pull fires. Range 0.1 to 0.9.",
  },
  {
    name: "vr_twoHandedWeapons",
    default: "1",
    platforms: VR_PLATFORMS,
    description:
      "How the weapon points when you hold the secondary grip with your off hand. Setup → VR Options → Controls → Two-Handed Weapons.",
    values: [
      { value: "0", meaning: "Off: primary hand only" },
      {
        value: "1",
        meaning:
          "Basic: the weapon aims along a line from your weapon hand toward your off hand",
      },
      {
        value: "2",
        meaning:
          "VR Gun Stock: the weapon anchors near your face, like sighting along a stock",
      },
    ],
  },
  {
    name: "vr_weaponScope",
    default: "1",
    platforms: VR_PLATFORMS,
    description:
      "Enable railgun zoom in VR. To zoom, raise the railgun to your face and hold the secondary grip with your off hand in front of it. Zoomed shots aim from your face through your off hand, whatever vr_twoHandedWeapons is set to.",
  },
  {
    name: "vr_worldscale",
    default: "32.0",
    platforms: VR_PLATFORMS,
    description:
      "Scale of the in-game world relative to your real-world body. The default, 32, puts 32 Quake units in a meter. Lower values shrink the world (you feel taller); higher values enlarge it.",
  },
];

// A dedicated server has no flatscreen or VR platform, so the Reference
// shows no platform tags for these.
export const SERVER_CVARS: CvarEntry[] = [
  {
    name: "bot_grapple",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Let bots use the grapple when the server hands it out (g_grapple), both to travel and to fight. Travel routes need Trinity navigation data (.aat files), which ships with the stock baseq3 and Team Arena maps; elsewhere bots still fight with the grapple.",
    values: [
      { value: "0", meaning: "Bots never use the grapple" },
      { value: "1", meaning: "Bots travel and fight with the grapple" },
      {
        value: "2",
        meaning: "As 1, logging bot grapple activity to the console",
      },
    ],
  },
  {
    name: "com_writeConfig",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Write archived cvars back to q3config_server.cfg. When several server instances run from one install, the write-back makes each inherit the others' archived values. trinity.cfg sets 0, so each server's own config file stays the only source of its settings.",
  },
  {
    name: "g_botColors",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "Give the stock bots their own effects colors (color1 and color2, the pair players pick for themselves) from scripts/botcolors.txt. Team Arena clan bots take their base bot's colors. To use your own list, ship a scripts/botcolors.txt in a pk3 whose name sorts after Trinity's pak8t.pk3 (pak3t.pk3 for Team Arena). Color codes in Customize covers what each color drives.",
    values: [
      {
        value: "0",
        meaning:
          "Bots use the colors from their bots.txt entry, or red and magenta when it has none",
      },
      { value: "1", meaning: "Bots listed in botcolors.txt use its colors" },
    ],
    notes: "Takes effect at the next map load, for bots added after it.",
  },
  {
    name: "g_clanRotation",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Winner-stays clan rotation in Team Arena: when a match ends, the losing team's clan rotates out (its roster bots swapped for the incoming clan's) and the next clan in teaminfo.txt order challenges the winner. It works only when g_redteam and g_blueteam name real Team Arena clans (Crusaders, Intruders, Pagans, Stroggs, The Fallen); custom team names leave teams alone.",
  },
  {
    name: "g_damage_gh",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      "Damage an attached grapple tether deals. Empty uses the game mode's value.",
  },
  {
    name: "g_knockback_gh",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      "Knockback the grapple pad delivers on impact. Empty uses the game mode's value.",
  },
  {
    name: "g_forceTeamColors",
    default: "1",
    platforms: ALL_PLATFORMS,
    description:
      "In team games, give every player on red or blue their team's color1, so rail cores and weapon glow match the team. color2, the rail spiral, stays each player's own. Players can still recolor teammates on their own screen with cg_teamColors.",
    notes: "Applies immediately to everyone connected; no map_restart needed.",
  },
  {
    name: "g_grapple",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Hand every player the grapple at spawn: a tenth weapon with unlimited ammo. Also offered as Grapple on the create-server and single-player menus.",
    notes: "Applies at each player's next spawn; no map_restart needed.",
  },
  {
    name: "g_logSync",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Write each game log line to disk immediately. The collector reads events in real time, so the Trinity network requires it.",
  },
  {
    name: "g_mode",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Game profile: the movement physics (acceleration, air control, friction) and the combat ruleset (per-weapon damage and knockback, ammo caps, armor mechanics, spawn health, and item respawn timing). Quake Live Turbo pairs Quake Live combat with CPMA-style air control. Also the Mode selector in the create-server and single-player menus.",
    values: [
      { value: "0", meaning: "Quake III Arena" },
      { value: "1", meaning: "CPMA" },
      { value: "2", meaning: "Quake Live" },
      { value: "3", meaning: "Quake Live Turbo" },
    ],
    notes: "Takes effect after a map_restart.",
  },
  {
    name: "g_overtimelimit",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Cap sudden-death overtime in minutes. 0 is unbounded, and a tied match can then produce huge demo files. Team modes like CTF often want more time than Tournament.",
  },
  {
    name: "g_rotation",
    default: "",
    platforms: ALL_PLATFORMS,
    description: "Path to a map rotation file for automated map cycling.",
  },
  {
    name: "g_teamDMSpawnThreshold",
    default: "8",
    platforms: ALL_PLATFORMS,
    description:
      "Use team and CTF spawn points on Team Deathmatch maps that have fewer FFA spawns than this value. Avoids telefrag-fests on small maps.",
  },
  {
    name: "g_trinityHandshake",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Admit only verified Trinity clients. Without it, the hub rejects match stats and hides the server from dashboard cards and the activity feed. With it, non-Trinity clients are disconnected after 10 seconds, and logged-in Trinity players have their account credentials checked as they connect, which links their GUID to their account with no !claim or !link.",
    notes: "Required (1) for the server to take part in the Trinity network.",
  },
  {
    name: "sv_conPort",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "The console tap's bound port, published in serverinfo so the collector finds it automatically. The engine sets it when sv_conTap is on.",
    notes: "Read-only: turn on the tap with sv_conTap.",
  },
  {
    name: "sv_conTap",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Give the server's operator and delegated admins a remote console: trinity console <server> sends rcon commands or follows the live console from anywhere they can log in, without shell access to the game box.",
    notes: "Required (1) for remote console access.",
  },
  {
    name: "sv_dlRate",
    default: "100",
    platforms: ALL_PLATFORMS,
    description:
      "Bandwidth for pk3 downloads over UDP, in kilobytes per second. It applies only to UDP downloads; HTTP downloads go through sv_dlURL.",
  },
  {
    name: "sv_dlURL",
    default: "",
    platforms: ALL_PLATFORMS,
    description:
      'Base URL for downloads of pk3s and TV demos. Required by sv_tvDownload. The bundled scripts/bootstrap-nginx.sh sets up a matching dl.<your-host> vhost with a Let\'s Encrypt certificate, and the wizard sets this to "https://dl.<your-host>" when it has a public URL.',
  },
  {
    name: "sv_tvAuto",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Record a TV demo on every map load. The Trinity network requires it, so matches on your server can be watched on the hub.",
  },
  {
    name: "sv_tvAutoMinPlayers",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "How many human players, not spectating, must be on at once to keep an automatic recording. 0 always keeps it, even for bot-only matches. Pair with sv_tvAutoMinPlayersSecs.",
  },
  {
    name: "sv_tvAutoMinPlayersSecs",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "How many seconds in a row the player threshold must be met to keep a recording. 0 means any moment counts. Pair with sv_tvAutoMinPlayers.",
  },
  {
    name: "sv_tvDownload",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Offer clients the match's TV recording over HTTP when the match ends. Requires sv_dlURL.",
  },
  {
    name: "sv_tvLive",
    default: "0",
    platforms: ALL_PLATFORMS,
    description:
      "Broadcast the in-progress match to the hub for live spectating.",
    notes: "Required (1) for the server to take part in the Trinity network.",
  },
  {
    name: "sv_tvpath",
    default: "demos",
    platforms: ALL_PLATFORMS,
    description:
      "Directory for TV recordings, relative to the active game folder: baseq3, or missionpack for Team Arena game types. The default, 'demos', writes .tvd files beside regular client demo recordings.",
  },
];
