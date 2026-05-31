import { useState } from "react";
import { Link } from "react-router-dom";
import { DocsH2 } from "./DocsH2";
import { DocsModeTabs } from "./DocsModeTabs";
import { PlatformOnly } from "./PlatformOnly";
import { PlatformNote } from "./PlatformNote";
import { CopyableCommand } from "./CopyableCommand";
import { ZoomableImage } from "./ZoomableImage";
import { CalloutImage, CalloutLegend, type CalloutData } from "./CalloutImage";
import { HitSoundPreview } from "./HitSoundPreview";
import { LazyVideo } from "./LazyVideo";
import { MOVEMENT_MODES } from "../ServerCard";

// Annotated-screenshot callout data. Each entry feeds both the
// CalloutImage overlay (hover/focus/tap tooltips on the image) and
// the legend list rendered below it — so prose lives in exactly one
// place. Coordinates are percentages of the SOURCE image dimensions
// (the full-size shots in files-for-play-guide/, sized to match the
// canvas the yellow boxes were drawn on); the overlay scales with
// the rendered image at any width.
//
// To author a new image's callouts: measure pixel rects against the
// source image (top-left x/y, width, height), divide x/w by source
// width and y/h by source height, multiply by 100. Source dims for
// hud-baseq3.png: 2560×1440.
const HUD_BASEQ3_CALLOUTS: CalloutData[] = [
  {
    n: 1,
    xPct: 35.156,
    yPct: 82.986,
    wPct: 18.828,
    hPct: 16.875,
    title: "Portrait + health",
    body: (
      <>
        Your character's head model paired with current health (yellow normally,
        red flashing at 1–25, white above 100 from Mega Health). The small
        speaker icon overlaid on the portrait shows your VOIP state — a red X
        means you're self-muted; a colored speaker indicates you're transmitting
        on that channel. See <Link to="#voice-chat">Voice chat</Link> for the
        channel colors.
      </>
    ),
  },
  {
    n: 2,
    xPct: 16.094,
    yPct: 88.125,
    wPct: 14.727,
    hPct: 11.875,
    title: "Ammo",
    body: (
      <>
        Your active weapon's ammo count, with the weapon-ammo icon next to it.
        Cycles to whichever weapon you're holding.
      </>
    ),
  },
  {
    n: 3,
    xPct: 57.109,
    yPct: 86.528,
    wPct: 16.367,
    hPct: 13.472,
    title: "Armor",
    body: (
      <>
        Current armor with the armor icon — yellow at 50 here. On CPMA gameplay
        (<code>g_gameplay 1</code>), the armor icon is color-coded green /
        yellow / red to reflect a tiered protection system, with different
        maximums and different absorption rates per tier. See{" "}
        <Link to="#modes">Movement &amp; gameplay variants</Link>.
      </>
    ),
  },
  {
    n: 4,
    xPct: 87.422,
    yPct: 79.236,
    wPct: 12.578,
    hPct: 10.833,
    title: "Score block",
    body: (
      <>
        Left to right: fraglimit, leader's score, your score (or 2nd place's if
        you're leading). The yellow outline anchors to <em>your</em> score so
        you can find yourself at a glance.
      </>
    ),
  },
  {
    n: 5,
    xPct: 0.078,
    yPct: 78.75,
    wPct: 12.148,
    hPct: 10.972,
    title: "Pickup label",
    body: (
      <>
        Briefly names whatever you just picked up — armor here, but it surfaces
        ammo, health, powerups, and weapons the same way. Fades after a few
        seconds.
      </>
    ),
  },
  {
    n: 6,
    xPct: 45.313,
    yPct: 33.958,
    wPct: 9.531,
    hPct: 7.708,
    title: "Crosshair name",
    body: (
      <>
        When your crosshair lands on a player, their name renders just above it.
      </>
    ),
  },
  {
    n: 7,
    xPct: 93.75,
    yPct: 0.069,
    wPct: 6.25,
    hPct: 4.583,
    title: "Timer + indicators",
    body: (
      <>
        Match clock at the top-right. Trinity counts <em>down</em> to the
        timelimit (vanilla counts up indefinitely), prefixes overtime with{" "}
        <code>OT</code>, and flashes red/white in the final 30 seconds of
        overtime. Other status indicators stack in this area, too.
      </>
    ),
  },
];

// Source dims for hud-teamarena.png: 2560×1440. The TA HUD goes
// through cg_newdraw.c (rectDef-driven) rather than cg_draw.c, so
// the layout is wholly different from baseq3 — many more callouts
// covering team-coordination chrome (medals, persistent powerups,
// teammate status, role declarations, lagometer).
const HUD_TEAMARENA_CALLOUTS: CalloutData[] = [
  {
    n: 1,
    xPct: 0,
    yPct: 0,
    wPct: 21.055,
    hPct: 15.694,
    title: "Console & voice chat",
    body: (
      <>
        Rolling notify area at the top-left — chat messages, kill feed entries,
        and server messages all interleave here. When a voice-chat macro is
        playing, a player portrait appears and shifts the text to the right.
        Grunt (a bot teammate) is announcing he's bringing the flag back to
        base.
      </>
    ),
  },
  {
    n: 2,
    xPct: 24.102,
    yPct: 10.694,
    wPct: 51.406,
    hPct: 10.764,
    title: "Medal display",
    body: (
      <>
        Pops in when you earn one, stacking the count if you've earned several
        of the same kind. Displays for a few seconds then fades out.
      </>
    ),
  },
  {
    n: 3,
    xPct: 88.945,
    yPct: 0,
    wPct: 11.055,
    hPct: 4.236,
    title: "Timer",
    body: (
      <>
        Same upper-right slot as vanilla Q3. The <code>OT</code> prefix means
        the match is in overtime.
      </>
    ),
  },
  {
    n: 4,
    xPct: 19.219,
    yPct: 29.097,
    wPct: 11.875,
    hPct: 28.542,
    title: "Flag",
    body: <>The blue flag on its stand — what you're defending in CTF.</>,
  },
  {
    n: 5,
    xPct: 37.383,
    yPct: 24.722,
    wPct: 25.82,
    hPct: 9.028,
    title: "Center-print notification",
    body: (
      <>
        Important messages appear here — frag confirmations, flag pickups, and
        other game events.
      </>
    ),
  },
  {
    n: 6,
    xPct: 42.813,
    yPct: 34.097,
    wPct: 3.242,
    hPct: 4.236,
    title: "Teammate indicator",
    body: (
      <>
        Downward-pointing yellow triangle that floats above teammates' heads —
        your "don't shoot, friend" cue.
      </>
    ),
  },
  {
    n: 7,
    xPct: 51.094,
    yPct: 38.542,
    wPct: 3.828,
    hPct: 7.431,
    title: "Persistent powerup",
    body: (
      <>
        One of four Team Arena <em>persistent</em> powerups sitting on its pad
        in the world — Guard, Ammo Regen, Scout, or Doubler (the red Doubler
        here). Once picked up, they stay with you for the rest of your life
        (until you die), unlike regular timed powerups.
      </>
    ),
  },
  {
    n: 8,
    xPct: 12.266,
    yPct: 75.486,
    wPct: 21.914,
    hPct: 14.167,
    title: "Teammate status card",
    body: (
      <>
        The game tries to surface a relevant teammate here — typically the flag
        carrier in CTF / 1FCTF, otherwise whoever's situationally interesting.
        Shows their name, current location, role-declaration icon, health,
        armor, and current weapon.
      </>
    ),
  },
  {
    n: 9,
    xPct: 12.109,
    yPct: 89.514,
    wPct: 15.547,
    hPct: 10.347,
    title: "Holdables / persistent powerup / carry status",
    body: (
      <>
        Three-slot indicator strip: what you're holding (Medkit, Personal
        Teleporter, Kamikaze, Invulnerability), which persistent powerup you've
        grabbed (Guard in this shot), and whether you're carrying a flag.
      </>
    ),
  },
  {
    n: 10,
    xPct: 27.773,
    yPct: 89.653,
    wPct: 14.648,
    hPct: 10.347,
    title: "Weapon + ammo",
    body: <>Active weapon icon and ammo count — Railgun with 35 slugs here.</>,
  },
  {
    n: 11,
    xPct: 42.93,
    yPct: 87.708,
    wPct: 14.531,
    hPct: 12.292,
    title: "Portrait + health",
    body: (
      <>
        Same conceptual element as in vanilla Q3, repositioned in the TA layout.
        The small speaker icon overlaid on the portrait shows your VOIP state —
        the cyan speaker here means you're on the team channel. See{" "}
        <Link to="#voice-chat">Voice chat</Link> for the channel colors.
      </>
    ),
  },
  {
    n: 12,
    xPct: 57.93,
    yPct: 89.375,
    wPct: 14.648,
    hPct: 10.625,
    title: "Armor",
    body: (
      <>
        Separate slot in the TA layout. Always visible — Team Arena renders the
        slot even at <code>0</code>, where vanilla Q3 just hides the armor
        element entirely until you pick some up.
      </>
    ),
  },
  {
    n: 13,
    xPct: 66.641,
    yPct: 82.361,
    wPct: 20.898,
    hPct: 7.014,
    title: "Status declaration + location",
    body: (
      <>
        What <em>you've</em> declared as your current role, with the matching
        task icon — the upward green arrow is <em>attack</em> — plus your
        current map location. Set via the <code>task*</code> commands listed
        below, or from the in-game <strong>Team Orders</strong> menu.
      </>
    ),
  },
  {
    n: 14,
    xPct: 72.656,
    yPct: 89.653,
    wPct: 14.883,
    hPct: 10.347,
    title: "Score block",
    body: (
      <>
        Capture limit, then each team's flag-state icon paired with their team
        score. The flag icons are the same set used on this site's live server
        cards. The smaller number under each is the <em>game score</em> — Team
        Arena tracks objective points separately from frags, with bigger
        objectives like captures worth more.
      </>
    ),
  },
  {
    n: 15,
    xPct: 94.102,
    yPct: 69.583,
    wPct: 5.898,
    hPct: 11.389,
    title: "Network graph",
    body: (
      <>
        Lagometer (<code>cg_lagometer 1</code>): ping in milliseconds plus a
        jitter trace below. A flat horizontal trace means a stable connection.
      </>
    ),
  },
];

// Source dims for scoreboard-baseq3.png: 2560×1440. Vanilla Q3
// scoreboard pulled up via Tab — much sparser than TA. The
// in-match status indicators (timer, score block, powerup timer)
// stay drawn underneath the scoreboard panel.
const SCOREBOARD_BASEQ3_CALLOUTS: CalloutData[] = [
  {
    n: 1,
    xPct: 21.094,
    yPct: 19.306,
    wPct: 11.055,
    hPct: 39.375,
    title: "Portrait column",
    body: (
      <>
        Each player's head model alongside their row. Status indicators to the
        left vary by player type: bot skill (difficulty 4 here), a gold star for
        verified admins, a green checkmark for verified accounts, or VR goggles
        behind the badge for VR clients. VOIP speaker icons appear at the
        bottom-right of each portrait, channel-colored and animated when
        speaking.
      </>
    ),
  },
  {
    n: 2,
    xPct: 38.516,
    yPct: 10.972,
    wPct: 22.969,
    hPct: 6.042,
    title: "Your placement",
    body: <>Centered headline showing the player's current rank and score.</>,
  },
  {
    n: 3,
    xPct: 87.852,
    yPct: 0,
    wPct: 12.148,
    hPct: 8.889,
    title: "Timer + mode icons",
    body: (
      <>
        The same upper-right stack as the in-match HUD. The two icons next to
        the timer report the active <em>movement</em> and <em>gameplay</em>{" "}
        profiles — both vanilla Q3 in this shot.
      </>
    ),
  },
  {
    n: 4,
    xPct: 86.328,
    yPct: 71.042,
    wPct: 13.672,
    hPct: 11.458,
    title: "Timed powerup",
    body: (
      <>
        Countdown for an active timed powerup — Quad Damage with 15 seconds
        remaining here. Appears whenever you're holding a timed powerup and
        fades when it expires.
      </>
    ),
  },
  {
    n: 5,
    xPct: 86.328,
    yPct: 82.431,
    wPct: 13.672,
    hPct: 5.903,
    title: "Persistent score indicator",
    body: (
      <>
        Same score block as the in-match HUD, kept on screen while the
        scoreboard is up. Yellow outline still anchors your score; the gray box
        keeps the leader's score visible for context.
      </>
    ),
  },
  {
    n: 6,
    xPct: 93.906,
    yPct: 89.236,
    wPct: 6.094,
    hPct: 10.764,
    title: "Network graph",
    body: (
      <>
        Lagometer (<code>cg_lagometer 1</code>): ping in milliseconds plus a
        jitter trace below. A flat horizontal trace means a stable connection.
      </>
    ),
  },
];

// Source dims for scoreboard-teamarena.png: 2560×1440. TA
// scoreboard is the densest annotated surface — team columns,
// per-player status (task) icons, leader designation, plus seven
// medal-category stat columns.
const SCOREBOARD_TEAMARENA_CALLOUTS: CalloutData[] = [
  {
    n: 1,
    xPct: 42.813,
    yPct: 16.875,
    wPct: 16.758,
    hPct: 5.486,
    title: "Team scores headline",
    body: <>Shows the current scores.</>,
  },
  {
    n: 2,
    xPct: 88.398,
    yPct: 0,
    wPct: 11.602,
    hPct: 8.472,
    title: "Timer + mode icons",
    body: (
      <>
        Same upper-right stack as the in-match HUD. The <code>OT</code> prefix
        means the match is in overtime. The two icons report the active{" "}
        <em>movement</em> and <em>gameplay</em> profiles — both vanilla Q3 in
        this shot.
      </>
    ),
  },
  {
    n: 3,
    xPct: 16.211,
    yPct: 30.139,
    wPct: 3.008,
    hPct: 17.986,
    title: "Status (task) column",
    body: (
      <>
        The role each player has declared via <code>task*</code> — attacking
        (green) or defending (red) here. Bots flip their declarations based on
        the team leader's orders; humans declare their own (and ignore orders if
        they like).
      </>
    ),
  },
  {
    n: 4,
    xPct: 50.078,
    yPct: 29.653,
    wPct: 3.125,
    hPct: 18.264,
    title: "Per-row indicators",
    body: (
      <>
        Each row carries a few glyphs: bot skill icon, or for humans, a gold
        star (admin), green checkmark (verified account), and VR goggles (VR
        client). Same concept as the vanilla Q3 portrait column but in the TA
        layout.
      </>
    ),
  },
  {
    n: 5,
    xPct: 54.336,
    yPct: 29.653,
    wPct: 5.43,
    hPct: 6.667,
    title: "Leader + VOIP",
    body: (
      <>
        One player per team can be designated team leader (a voteable status).
        The leader can issue orders that bots respect. The VOIP speaker icon
        (cyan here = team channel) shows who's speaking on what channel while
        the scoreboard is up.
      </>
    ),
  },
  {
    n: 6,
    xPct: 14.336,
    yPct: 75.972,
    wPct: 71.992,
    hPct: 10.764,
    title: "Performance stats",
    body: (
      <>
        Left to right: accuracy %, assists, defends, excellents, humiliations,
        impressives, captures. These are the medal categories — every player
        accumulates them across the match.
      </>
    ),
  },
  {
    n: 7,
    xPct: 94.023,
    yPct: 69.514,
    wPct: 5.977,
    hPct: 11.181,
    title: "Network graph",
    body: (
      <>
        Lagometer (<code>cg_lagometer 1</code>): ping in milliseconds plus a
        jitter trace below. A flat horizontal trace means a stable connection.
      </>
    ),
  },
];

// Source dims for demo-player.png: 1467×991 (smaller than the
// 2560×1440 in-game shots — captured from the web demo player
// rather than the engine's framebuffer).
const DEMO_PLAYER_CALLOUTS: CalloutData[] = [
  {
    n: 1,
    xPct: 14.315,
    yPct: 17.154,
    wPct: 14.519,
    hPct: 41.171,
    title: "VR badge in the handicap column",
    body: (
      <>
        The same scoreboard cell that shows bot difficulty for AI players
        surfaces a <code>[VR]</code> badge for VR clients. VR being a difficulty
        modifier in its own right, this lets you read who's on which kind of
        setup at a glance.
      </>
    ),
  },
  {
    n: 2,
    xPct: 81.595,
    yPct: 0,
    wPct: 18.405,
    hPct: 17.356,
    title: "Followed-player VR + recent speakers",
    body: (
      <>
        Top-right shows whether the currently-followed player is on VR, plus a
        list of recently-active VOIP speakers (channel-colored). Inactive
        entries fade out, so the list only shows people who are speaking or just
        spoke.
      </>
    ),
  },
  {
    n: 3,
    xPct: 3.408,
    yPct: 89.304,
    wPct: 92.025,
    hPct: 10.696,
    title: "Playback timeline",
    body: (
      <>
        Yellow scrub bar at the very bottom of the canvas, with current time /
        total duration in the corner. In your local client, hold a key bound to{" "}
        <code>+tv_scrub</code> to drag the scrub indicator and jump to a
        specific moment.
      </>
    ),
  },
];

// /docs/play — Player's Guide. Two layers of coverage:
//   1. Quake 3 basics for players arriving from VR communities or
//      the landing page who haven't played Q3 before. Concise; vets
//      skim past in seconds.
//   2. What Trinity adds on top — accounts, optional client polish,
//      orbit + scrubbable spectating, voice chat, and (on VR builds)
//      head/weapon tracking and grip-based weapon adjustment.
//
// Per docs principles: every menu path and cvar surface here is
// verified against the relevant engine source — flatscreen items
// against ../trinity-engine + ../trinity (mod), VR items against
// ../trinity-vr (PCVR) or ../trinity-quest (Quest). Cvar enumeration belongs
// on /docs/reference, not here — this tab describes; Reference lists.
//
// Platform divergence is expressed as paired PlatformOnly blocks
// (one per platform variant). Reasoning: the reader's question is
// "how do I do this on my client?", not "how does it differ?", so
// they see one variant at a time and flip the rail badge if they
// want to peek at another. PlatformTabs is reserved for sections
// where seeing the choice up front is the point (downloads on
// /docs/install — not used here).
const VOIP_CHANNELS = [
  { key: "spatial", label: "Spatial", color: "rgb(255, 255, 51)" },
  { key: "team", label: "Team", color: "rgb(51, 255, 255)" },
  { key: "all", label: "All", color: "rgb(51, 255, 51)" },
  { key: "direct", label: "Direct", color: "rgb(255, 51, 255)" },
] as const;

const VOIP_LEVELS = [
  { suffix: "muted", label: "Muted", bright: false },
  { suffix: "idle", label: "Idle", bright: false },
  { suffix: "level1", label: "< 30%", bright: false },
  { suffix: "level2", label: "< 60%", bright: false },
  { suffix: "level3", label: "< 90%", bright: false },
  { suffix: "level4", label: "90%+", bright: true },
] as const;

function VoipLevelStrip() {
  const [channel, setChannel] = useState<string>("spatial");
  return (
    <div className="voip-level-panel">
      <div className="voip-level-tabs" role="tablist">
        {VOIP_CHANNELS.map((c) => (
          <button
            key={c.key}
            role="tab"
            aria-selected={channel === c.key}
            className={`voip-level-tab voip-level-tab--${c.key}${channel === c.key ? " voip-level-tab--active" : ""}`}
            onClick={() => setChannel(c.key)}
          >
            {c.label}
          </button>
        ))}
      </div>
      <div className="voip-level-strip">
        {VOIP_LEVELS.map((lv) => (
          <div
            key={lv.suffix}
            className={`voip-level-item${lv.bright ? " voip-level-item--bright" : ""}`}
          >
            <img
              src={`/assets/play/voip/speaker-${lv.suffix}-${channel}.png`}
              alt={`${lv.label} (${channel})`}
            />
            <span>{lv.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function DocsPlay() {
  return (
    <>
      <div className="about-section docs-play__hero">
        <h1 className="docs-play__title">Playing Trinity</h1>
        <p className="docs-play__lead">
          What your Trinity client does on top of vanilla Quake 3 — visual
          feedback, voice chat, demo recording and playback, persistent
          accounts, and (on VR builds) full motion tracking. Trinity stays
          compatible with vanilla Q3 servers, so installing it doesn't change
          which servers you can join.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="q3-basics">Quake 3 basics</DocsH2>
        <details className="docs-collapsible">
          <summary>
            For new players — goal, game types, weapons, items. Skip if you've
            played Quake 3 before.
          </summary>

          <h3 className="docs-play__feature-title">Goal &amp; format</h3>
          <p>
            Most matches share a format: a time limit, a score limit, and
            whichever fires first ends the match — the player or team in the
            lead at that moment wins. Servers usually advertise both — say,{" "}
            <em>20 minutes or 30 frags</em>. What counts as "score" depends on
            the game type, though. Free For All, Tournament, and Team Deathmatch
            are frag-driven; Capture the Flag and the Team Arena modes have
            their own objective-based scoring.
          </p>

          <h3 className="docs-play__feature-title">Game types</h3>
          <p>
            Servers run one game type at a time. Cards on{" "}
            <Link to="/servers">Servers</Link> show which:
          </p>
          <ul>
            <li>
              <strong>Free For All</strong> — everyone vs. everyone.
            </li>
            <li>
              <strong>Tournament</strong> — 1v1.
            </li>
            <li>
              <strong>Team Deathmatch</strong> — team vs. team, frag count.
            </li>
            <li>
              <strong>Capture the Flag</strong> — grab their flag, return yours,
              score.
            </li>
            <li>
              <strong>One-Flag CTF</strong> <em>(Team Arena)</em> — one neutral
              flag, deliver to the enemy base.
            </li>
            <li>
              <strong>Overload</strong> <em>(Team Arena)</em> — destroy the
              enemy obelisk.
            </li>
            <li>
              <strong>Harvester</strong> <em>(Team Arena)</em> — collect skulls
              from kills, deposit at the enemy receptacle.
            </li>
          </ul>

          <h3 className="docs-play__feature-title">Weapons</h3>
          <p>
            Q3 ships nine weapons; Team Arena adds three more. Pick them up by
            walking over them.
          </p>
          <PlatformOnly platform="flatscreen">
            <p>Switch using keybinds or the scroll wheel.</p>
          </PlatformOnly>
          <PlatformOnly platform={["pcvr", "quest"]}>
            <p>
              Switch via the weapon wheel — hold the primary grip to bring it
              up, point at the slot you want, release to select. The Trinity
              starter VR autoexec wires this by default.
            </p>
          </PlatformOnly>
          <ul>
            <li>
              <strong>Gauntlet</strong> — chainsaw melee. Killing with it
              triggers a humiliation indicator in the kill feed.
            </li>
            <li>
              <strong>Machinegun</strong> — accurate hitscan, light damage per
              round.
            </li>
            <li>
              <strong>Shotgun</strong> — close-range pellet spread.
            </li>
            <li>
              <strong>Grenade Launcher</strong> — bouncing arcing projectiles.
            </li>
            <li>
              <strong>Rocket Launcher</strong> — splash damage; rocket-jumping
              fuel.
            </li>
            <li>
              <strong>Lightning Gun</strong> — short-range continuous beam.
            </li>
            <li>
              <strong>Railgun</strong> — slow-firing instant-hit sniper.
            </li>
            <li>
              <strong>Plasma Gun</strong> — rapid-fire energy projectiles.
            </li>
            <li>
              <strong>BFG10K</strong> — superweapon; rare placement.
            </li>
            <li>
              <strong>Nailgun</strong> <em>(Team Arena)</em> — massive damage at
              close range, but the spread and projectile speed make it tricky at
              a distance.
            </li>
            <li>
              <strong>Prox Launcher</strong> <em>(Team Arena)</em> — lays
              proximity mines that detonate when an enemy gets close. Area
              denial.
            </li>
            <li>
              <strong>Chaingun</strong> <em>(Team Arena)</em> — belt-fed
              rapid-fire bullet weapon; sustained DPS.
            </li>
          </ul>

          <h3 className="docs-play__feature-title">Items</h3>
          <p>
            Pickups respawn on timers that depend on game mode and the server's
            gameplay setting.
          </p>
          <ul>
            <li>
              <strong>Armor</strong> — in vanilla Quake 3: Shard (+5), Yellow
              (+50), Heavy/Red (+100), maxing at 200. CPMA replaces this with a
              tiered system (lower-tier green armor on top of yellow and red,
              with tier-dependent absorption); see{" "}
              <Link to="/docs/reference#server-cvars">
                Reference · Server CVars
              </Link>{" "}
              entry for <code>g_gameplay</code>.
            </li>
            <li>
              <strong>Health</strong> — 5, 25, 50, and Mega Health (+100,
              overstacks past your normal cap).
            </li>
            <li>
              <strong>Powerups</strong> — Quad Damage, Battle Suit, Speed
              (Haste), Regeneration, Invisibility (all 30 seconds); Flight (60
              seconds).
            </li>
            <li>
              <strong>Persistent powerups</strong> <em>(Team Arena)</em> —
              Guard, Ammo Regen, Scout, and Doubler. Unlike the timed powerups
              above, these have no countdown: once picked up they stay with you
              for the rest of your life, until you die.
            </li>
            <li>
              <strong>Holdables</strong> (use when ready) — Personal Teleporter,
              Medkit. Team Arena adds Kamikaze and Invulnerability.
            </li>
          </ul>
        </details>
      </div>

      <div className="about-section">
        <DocsH2 id="accounts">Trinity accounts</DocsH2>
        <p>
          Trinity records the matches you play on participating servers. Without
          an account, those records attach to the per-install identity Quake 3
          generates (your <code>qkey</code>); each new install starts a new
          identity from the hub's perspective.
        </p>
        <p>
          A Trinity account ties every install you sign in on to one record —
          same name on leaderboards, same stats, no manual <code>!link</code>{" "}
          chasing. Setup is one in-game command and a quick visit to the hub:{" "}
          <Link to="/docs/account">Account</Link> walks through the flow.
        </p>
        <p>
          To find Trinity-collected servers in-game, open{" "}
          <strong>Multiplayer</strong> from the main menu, then click the{" "}
          <em>Servers:</em> selector at the top until it reads{" "}
          <strong>Trinity</strong> — that filters the list to servers registered
          with the hub. The same servers are also listed on this site, with live
          status, at <Link to="/servers">Servers</Link>.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="voice-chat">Voice chat</DocsH2>
        <p>
          Opus-based voice chat works on supported servers and is recorded into
          TV demos so spectators can hear the chatter on playback. Three
          transmission targets, plus a fourth off-cycle option:
        </p>
        <ul>
          <li>
            <strong>spatial</strong> — everyone nearby hears you, attenuated
            with distance.
          </li>
          <li>
            <strong>team</strong> — your team only (team game modes only).
          </li>
          <li>
            <strong>all</strong> — every player on the server hears you,
            regardless of team or position.
          </li>
          <li>
            <strong>direct</strong> — a comma-separated list of player IDs (or
            the keywords <code>attacker</code> / <code>crosshair</code>). Set on{" "}
            <code>cl_voipSendTarget</code> directly; not part of the cycle
            below.
          </li>
        </ul>
        <p>
          The <code>voiptarget</code> command cycles between the first three in
          order — <em>spatial → team (when applicable) → all → spatial</em>.
          Bind a one-shot <em>set cl_voipSendTarget …</em> if you want a hot key
          for whispering to a particular teammate.
        </p>

        <h3 className="docs-play__feature-title">Spotting who's talking</h3>
        <p>
          Trinity surfaces voice activity in three places visible during play,
          using channel-colored speaker icons:
        </p>
        <ul>
          <li>
            <strong>Player portrait icon</strong> — a speaker icon on your own
            portrait shows when your mic is active and transmitting.
          </li>
          <li>
            <strong>Upper-right speakers list</strong> —{" "}
            <code>cg_drawVoipSpeakers 1</code> draws a stack of
            currently-speaking player names in the upper-right of the HUD.
          </li>
          <li>
            <strong>Over-head indicators</strong> — in-world speaker icons
            appear above speaking players' models, so you know who's talking
            even when you can't see (or aren't looking at) the HUD list.
          </li>
        </ul>
        <p>
          The number of arcs on each icon reflects the speaker's peak volume —
          more arcs means a higher peak, and at 90%+, the icon brightens.
        </p>
        <VoipLevelStrip />

        <h3 className="docs-play__feature-title">Activating voice chat</h3>
        <p>Two ways to control when your mic transmits:</p>
        <div className="docs-table-scroll">
          <table className="docs-color-table">
            <thead>
              <tr>
                <th></th>
                <th>Push-to-talk</th>
                <th>Voice activity (VAD)</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <th scope="row">Mode cvar</th>
                <td>
                  <code>cl_voipUseVAD 0</code>
                </td>
                <td>
                  <code>cl_voipUseVAD 1</code>
                </td>
              </tr>
              <tr>
                <th scope="row">When mic transmits</th>
                <td>only while you hold a bound input</td>
                <td>
                  whenever the mic picks up sound above{" "}
                  <code>cl_voipVADThreshold</code>
                </td>
              </tr>
              <tr>
                <th scope="row">Bind to</th>
                <td>
                  <code>+voiprecord</code> — held while speaking
                </td>
                <td>
                  <code>voipvadtoggle</code> — toggles the mic on / off
                </td>
              </tr>
              <tr>
                <th scope="row">Best for</th>
                <td>noisy rooms, controlled chatter</td>
                <td>quiet rooms, good noise-cancelling mic</td>
              </tr>
            </tbody>
          </table>
        </div>

        <p>Where to put the bind:</p>
        <PlatformOnly platform="flatscreen">
          <p>
            Trinity's starter autoexec uses <code>Q</code> as a VAD toggle. Swap
            the bound command if you'd rather do push-to-talk:
          </p>
          <CopyableCommand>{`bind q "voipvadtoggle"     // VAD: tap to mute/unmute (starter default)
// bind q "+voiprecord"     // PTT: hold to talk
bind e "voiptarget"        // cycle channel: spatial → team → all`}</CopyableCommand>
        </PlatformOnly>
        <PlatformOnly platform={["pcvr", "quest"]}>
          <p>
            The natural home is the <strong>primary thumbstick press</strong> —
            unbound by default and out of the way of weapon buttons. Trinity's
            starter VR autoexec wires the thumbrests to <code>+alt</code>, which
            gives every other button a second action when you rest your thumb on
            the controller. Pair them like this:
          </p>
          <CopyableCommand>{`seta vr_button_map_PRIMARYTHUMBSTICK "+voiprecord"          // PTT: hold to talk
// seta vr_button_map_PRIMARYTHUMBSTICK "voipvadtoggle"     // VAD: tap to mute/unmute
seta vr_button_map_PRIMARYTHUMBSTICK_ALT "voiptarget"       // hold thumbrest + click stick to cycle channel`}</CopyableCommand>
        </PlatformOnly>

        <p>
          Voice chat playback volume lives in{" "}
          <strong>Setup → System → Sound</strong> (the <em>VOIP Volume</em>{" "}
          slider) on every client. The rest of the voice surface — VAD
          threshold, channel mutes, the on-HUD speaker list, and the engine mic
          meter — is autoexec-only;{" "}
          <Link to="/docs/reference#player-cvars">
            Reference · Player CVars
          </Link>{" "}
          lists the full set.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="vr-tracking">1:1 head and weapon tracking</DocsH2>
        <p>
          Trinity's VR builds track player head movement and weapon aim 1:1, so
          what you see in-game reflects how the player is actually moving. It's
          subtle, but it's visible to all players, flatscreen and VR alike — in
          live gameplay and demo playback.
        </p>
        <p>Nothing to enable — it works automatically on any Trinity server.</p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/vr_tracking.webm"
            mp4="/assets/play/vr_tracking.mp4"
          />
          <figcaption>
            A VR player's head pose and weapon-aim direction tracked in real
            time — the same motion every other player in the match (flatscreen
            or VR) sees on their screen.
          </figcaption>
        </figure>
      </div>

      <PlatformOnly platform={["pcvr", "quest"]}>
        <div className="about-section">
          <DocsH2 id="vr" platforms={["pcvr", "quest"]}>
            VR-specific features
          </DocsH2>

          <h3 className="docs-play__feature-title">
            Two-handed weapon grip <code>vr_twoHandedWeapons</code>
          </h3>
          <p>
            Hold the secondary grip to stabilize the weapon. Two modes, set in{" "}
            <strong>Setup → Controls → Two-Handed Weapons</strong> (or with{" "}
            <code>vr_twoHandedWeapons</code> in autoexec):
          </p>
          <ul>
            <li>
              <strong>Basic</strong> — weapon aims along a line from your
              primary controller toward your secondary controller.
            </li>
            <li>
              <strong>Virtual gun stock</strong> — weapon anchors near your
              face, like sighting along a stock.
            </li>
          </ul>

          <h3 className="docs-play__feature-title">
            In-game weapon position adjustment <code>vr_weaponAdjust</code>
          </h3>
          <p>
            Trinity lets you tune how each weapon sits in your hand without
            leaving the game — useful because the "right" position depends on
            any weapon model replacements you use, on your VR controller's
            tracking behavior, and on personal preference. Enable the feature
            first:
          </p>
          <PlatformOnly platform="pcvr">
            <p>
              Toggle <strong>Setup → VR Options → Weapon Adjustment</strong>, or
              set <code>vr_weaponAdjust 1</code> in autoexec.
            </p>
          </PlatformOnly>
          <PlatformOnly platform="quest">
            <p>
              Toggle <strong>Setup → Controls → Weapon Adjustment</strong>, or
              set <code>vr_weaponAdjust 1</code> in autoexec.
            </p>
          </PlatformOnly>
          <p>
            Then in-match,{" "}
            <strong>hold both grip triggers together for one second</strong>{" "}
            (controller buzzes when it fires) to enter adjustment mode. A
            parameter strip appears at the top of your view with seven values
            you can tune for the current weapon: <em>scale</em>, <em>right</em>,{" "}
            <em>up</em>, <em>fwd</em>, <em>pitch</em>, <em>yaw</em>, and{" "}
            <em>roll</em>.
          </p>
          <figure className="docs-hud-figure">
            <ZoomableImage
              src="/assets/play/weapon-adjust.jpg"
              alt="In-VR view of weapon adjustment mode: the parameter strip across the top of the screen lists scale, right, up, fwd, pitch, yaw, and roll, with the currently-selected parameter highlighted."
            />
            <figcaption>
              The parameter strip across the top of your view in adjustment
              mode. The secondary thumbstick cycles which value is selected; the
              primary thumbstick tunes it.
            </figcaption>
          </figure>
          <ul>
            <li>
              <strong>Secondary thumbstick left / right</strong> — cycles which
              parameter is selected.
            </li>
            <li>
              <strong>Primary thumbstick up / down</strong> — adjusts the
              selected value (a fine-control curve gives small deflections
              precise nudges).
            </li>
            <li>
              <strong>A button</strong> — accept and exit adjustment mode.
            </li>
            <li>
              <strong>B button</strong> — reset to default: <em>tap</em> resets
              just the selected parameter; <em>hold for two seconds</em> resets
              every parameter on the current weapon (controller buzzes when the
              full reset fires).
            </li>
          </ul>
          <p>
            Switch weapons mid-adjustment to tune another one — the strip
            reloads automatically. Changes write back to the per-weapon offset
            cvars (<code>vr_weapon_adjustment_N</code>, one per weapon slot) the
            moment you make them, so they persist across sessions. Adjustment
            mode auto-exits if you die, open the virtual screen, scope a weapon,
            or start spectating.
          </p>

          <h3 className="docs-play__feature-title">Comfort options</h3>
          <p>
            Snap turn, comfort vignette, height adjust, smooth spectator camera,
            haptic intensity, HUD depth / scale / Y-offset, and the
            single-player 6DoF toggle all live in{" "}
            <strong>Setup → Comfort Options</strong>, with sliders so you can
            tune by feel. The{" "}
            <Link to="/docs/reference#vr-cvars">VR CVars</Link> reference lists
            the underlying cvar names if you'd rather set defaults from{" "}
            <code>autoexec.cfg</code>.
          </p>
          <PlatformNote platform="quest">
            <p>
              Quest also exposes <strong>Refresh Rate</strong> and{" "}
              <strong>Supersampling</strong> under{" "}
              <strong>Setup → System → Graphics</strong>. Bumping refresh up (90
              or 120) and supersampling above 1.0 both cost frame budget — tune
              one, then check the other.
            </p>
          </PlatformNote>

          <h3 className="docs-play__feature-title">VR controls</h3>
          <p>
            Direction mode (head vs secondary controller reference), snap turn
            angle, U-turn, handedness, weapon scope (railgun zoom), trigger
            sensitivity, control schema, and the thumbstick swap all live in{" "}
            <strong>Setup → Controls</strong>. Per-button assignments live in
            the <code>vr_button_map_*</code> entries in your starter autoexec —
            see <Link to="/docs/customize">Customize</Link>.
          </p>

          <h3 className="docs-play__feature-title">VR keyboard</h3>
          <p>
            When you need to type — chat messages, console commands, name fields
            — Trinity opens a virtual keyboard with two cursors, one red and one
            blue, one per controller. Aim a cursor at a key and pull that
            controller's trigger to press it. Alternating between cursors is
            much faster than pecking letter-by-letter with one.
          </p>
          <figure className="docs-hud-figure">
            <ZoomableImage
              src="/assets/play/vr-keyboard.jpg"
              alt="The Trinity VR virtual keyboard floating in front of the player, with a red cursor from one controller and a blue cursor from the other, both pointing at keys."
            />
            <figcaption>
              Two cursors, one per controller — type by aiming and pulling the
              matching trigger.
            </figcaption>
          </figure>

          <h3 className="docs-play__feature-title">
            VR shows on the scoreboard as a handicap
          </h3>
          <p>
            Trinity puts a VR icon (
            <img
              src="/assets/vr/vr.png"
              alt="VR icon"
              className="docs-inline-icon"
            />
            ) in the handicap slot on the scoreboard for players connected from
            a VR client. Playing Quake 3 in VR demands more of the player than
            flatscreen does — the icon tells flatscreen opponents who they're up
            against. The same icon shows up when you follow a VR player as a
            spectator.
          </p>
        </div>
      </PlatformOnly>

      <div className="about-section">
        <DocsH2 id="hud">HUD</DocsH2>
        <p>
          What's on screen during a match. The layout depends on whether the
          server is running vanilla Quake III or Team Arena (Q3's missionpack
          expansion) — Team Arena packs in more objective and team-coordination
          chrome.
        </p>
        <p>
          Each screenshot below is one moment from one match — it can't show
          every possible state of every element, but the layouts hold across
          matches, so the callouts should orient a new player to where things
          live. Toggle between modes:
        </p>
        <DocsModeTabs>
          <DocsModeTabs.Panel mode="baseq3">
            <figure className="docs-hud-figure">
              <CalloutImage
                src="/assets/play/hud-baseq3.png"
                alt="Quake III HUD with seven numbered callouts: portrait + health, ammo, armor, score block, pickup label, crosshair name, and timer."
                callouts={HUD_BASEQ3_CALLOUTS}
              />
            </figure>
            <CalloutLegend callouts={HUD_BASEQ3_CALLOUTS} />
            <p>
              The red vignette around the edges of this screenshot is the{" "}
              <Link to="#feedback">
                <code>cg_damageEffect</code>
              </Link>{" "}
              directional damage indicator firing — not a HUD element itself,
              but you'll see it constantly in matches.
            </p>
          </DocsModeTabs.Panel>

          <DocsModeTabs.Panel mode="teamarena">
            <figure className="docs-hud-figure">
              <CalloutImage
                src="/assets/play/hud-teamarena.png"
                alt="Team Arena HUD with fifteen numbered callouts: console and voice chat, medal display, timer, flag, center-print notification, teammate indicator, persistent powerup, teammate status card, holdables and powerup and carry slots, weapon, portrait, armor, status declaration and location, score block, and lagometer."
                callouts={HUD_TEAMARENA_CALLOUTS}
              />
            </figure>
            <CalloutLegend callouts={HUD_TEAMARENA_CALLOUTS} />

            <p>
              <strong>Team Arena role declarations.</strong> Each player can
              declare what they're doing for the team via one of these console
              commands (or from the in-game <strong>Team Orders</strong> menu).
              The declaration shows up on teammates' HUDs alongside the player's
              location:
            </p>
            <ul className="docs-cvars">
              <li>
                <code>taskOffense</code> — <em>attack</em>; says "I'm on
                offense" (or "going for the flag" in CTF).
              </li>
              <li>
                <code>taskDefense</code> — <em>defend</em>; says "I'm on
                defense".
              </li>
              <li>
                <code>taskPatrol</code> — <em>patrol</em>; says "I'm
                patrolling".
              </li>
              <li>
                <code>taskCamp</code> — <em>camp</em>; says "I'm camping here".
              </li>
              <li>
                <code>taskFollow</code> — <em>follow</em>; says "following the
                leader".
              </li>
              <li>
                <code>taskRetrieve</code> — <em>retrieve</em>; says "going to
                return the flag".
              </li>
              <li>
                <code>taskEscort</code> — <em>escort</em>; says "escorting the
                flag carrier".
              </li>
              <li>
                <code>taskOwnFlag</code> — voice-only; says "I have the flag"
                without changing your task.
              </li>
            </ul>
            <p>
              The whole role system is from id Software's Team Arena — you'll
              see it on any TA server, Trinity-collected or otherwise. Useful
              for coordinating without typing.
            </p>
          </DocsModeTabs.Panel>
        </DocsModeTabs>
      </div>

      <div className="about-section">
        <DocsH2 id="scoreboard">Scoreboard</DocsH2>
        <p>
          See who's playing and how everyone's doing during a match.{" "}
          <PlatformOnly platform="flatscreen">
            Hold <code>Tab</code> to pull it up.
          </PlatformOnly>
          <PlatformOnly platform={["pcvr", "quest"]}>
            Press the secondary thumbstick to pull it up — the starter VR
            autoexec wires <code>+scores</code> to{" "}
            <code>vr_button_map_SECONDARYTHUMBSTICK</code>.
          </PlatformOnly>
        </p>
        <p>
          Like the HUD, the layout depends on whether the server is running
          vanilla Quake III or Team Arena.
        </p>
        <DocsModeTabs>
          <DocsModeTabs.Panel mode="baseq3">
            <figure className="docs-hud-figure">
              <CalloutImage
                src="/assets/play/scoreboard-baseq3.png"
                alt="Quake III scoreboard with six numbered callouts: portrait column, placement headline, timer with mode icons, timed powerup countdown, persistent score indicator, and lagometer."
                callouts={SCOREBOARD_BASEQ3_CALLOUTS}
              />
            </figure>
            <CalloutLegend callouts={SCOREBOARD_BASEQ3_CALLOUTS} />
          </DocsModeTabs.Panel>

          <DocsModeTabs.Panel mode="teamarena">
            <figure className="docs-hud-figure">
              <CalloutImage
                src="/assets/play/scoreboard-teamarena.png"
                alt="Team Arena scoreboard with seven numbered callouts: team scores headline, timer with mode icons, status (task) column, per-row indicators, leader and VOIP, performance stats, and lagometer."
                callouts={SCOREBOARD_TEAMARENA_CALLOUTS}
              />
            </figure>
            <CalloutLegend callouts={SCOREBOARD_TEAMARENA_CALLOUTS} />
          </DocsModeTabs.Panel>
        </DocsModeTabs>
      </div>

      <div className="about-section">
        <DocsH2 id="modes">Movement &amp; gameplay variants</DocsH2>
        <p>
          Trinity ships four movement profiles and three gameplay profiles —
          Vanilla Q3, CPMA, Quake Live, and (movement only) Quake Live Turbo.
          Servers pick the rules; you can also pick them locally for
          single-player against bots, under{" "}
          <strong>Setup → Game Options</strong> (the <em>Movement</em> and{" "}
          <em>Gameplay</em> dropdowns near the bottom).
        </p>
        <p>The icons you'll see on server cards and scoreboards:</p>
        <div className="docs-modes">
          {Object.values(MOVEMENT_MODES).map((m) => (
            <div key={m.icon} className="docs-mode-item">
              <img src={m.icon} alt={m.label} className="docs-mode-icon" />
              <div className="docs-mode-info">
                <span className="docs-mode-name">{m.label}</span>
              </div>
            </div>
          ))}
        </div>
        <p>
          Joining a multiplayer server hands movement and gameplay over to
          whatever it's running. For how local choices interact with
          server-enforced rules, see{" "}
          <Link to="/docs/customize#modes-note">
            Customize · Movement &amp; gameplay modes
          </Link>
          .
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="spectating-demos">Spectating and demos</DocsH2>

        <h3 className="docs-play__feature-title">
          Orbit camera while spectating
        </h3>
        <p>
          When you're following another player, switch from first-person to a
          third-person camera that orbits around them.
        </p>
        <PlatformOnly platform="flatscreen">
          <p>
            Pick <em>First-person</em> or <em>Orbit</em> under{" "}
            <strong>Setup → Game Options → Follow Camera</strong>, or set{" "}
            <code>cg_followMode 1</code> in autoexec. A third <em>free-fly</em>{" "}
            mode (<code>cg_followMode 2</code>) is available during TV demo
            playback only — the menu doesn't expose it because it doesn't make
            sense mid-match.
          </p>
        </PlatformOnly>
        <PlatformOnly platform={["pcvr", "quest"]}>
          <p>
            Toggle <strong>Setup → Comfort Options → Smooth Follow</strong> (or
            set <code>cg_smoothFollow 1</code> in autoexec). Once it's on, your
            primary thumbstick orbits the camera around the player you're
            following. It lives in Comfort Options because continuous camera
            movement in VR takes strong VR legs.
          </p>
        </PlatformOnly>

        <h3 className="docs-play__feature-title">Scrub and pause TV demos</h3>
        <p>
          During TV demo playback, hold a key (or controller button) bound to{" "}
          <code>+tv_scrub</code> to scrub forward and backward through the
          timeline. Bind <code>demopause</code> to freeze a moment in time. Both
          are bind targets — set them up in your <code>autoexec.cfg</code> the
          same way you'd bind any input.
        </p>

        <h3 className="docs-play__feature-title">
          Replay matches in the browser or in your client
        </h3>
        <p>
          Trinity records every match on participating servers as a TVD
          (TrinityVision Demo) file. The hub keeps the files and offers them two
          ways:
        </p>
        <ul>
          <li>
            <strong>In the browser</strong> — open any match from{" "}
            <Link to="/matches">/matches</Link> and the hub plays it back
            through the Trinity engine compiled to WebAssembly. No install
            required, and the link works for anyone you share it with.
          </li>
          <li>
            <strong>In your local game client</strong> — download the{" "}
            <code>.tvd</code> file from the match page (or accept the post-match
            download prompt, which writes it into your demos folder). Local
            playback gives you the full <code>+tv_scrub</code> and{" "}
            <code>demopause</code> controls described above.
          </li>
        </ul>

        <figure className="docs-hud-figure">
          <CalloutImage
            src="/assets/play/demo-player.png"
            alt="Trinity web demo player showing scoreboard with VR badges in the handicap column, recent VOIP speakers in the upper-right, and the playback timeline scrubber at the bottom."
            callouts={DEMO_PLAYER_CALLOUTS}
          />
          <figcaption>
            Watching a Trinity match in the browser — same in-game scoreboard,
            plus the playback timeline at the bottom.
          </figcaption>
        </figure>
        <CalloutLegend callouts={DEMO_PLAYER_CALLOUTS} />

        <p>
          The post-match download prompt's behavior is set with{" "}
          <code>cl_tvDownload</code> (Off / Decline / Accept):
        </p>
        <PlatformOnly platform="flatscreen">
          <p>
            Pick the value under{" "}
            <strong>Setup → Game Options → TVD Download</strong>, or set{" "}
            <code>cl_tvDownload</code> in autoexec.
          </p>
        </PlatformOnly>
        <PlatformOnly platform={["pcvr", "quest"]}>
          <p>
            Pick the value under{" "}
            <strong>Setup → System → Network → TVD Download</strong>, or set{" "}
            <code>cl_tvDownload</code> in autoexec.
          </p>
        </PlatformOnly>
        <p>
          The full value table lives on{" "}
          <Link to="/docs/reference#player-cvars">
            Reference · Player CVars
          </Link>
          .
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="feedback">Visual &amp; audio feedback</DocsH2>

        <h3 className="docs-play__feature-title">
          Damage Plums <code>cg_damagePlums</code>
        </h3>
        <p>
          Floating damage numbers above each hit, telling you exactly how much
          damage you dealt.
        </p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/cg_damagePlums.webm"
            mp4="/assets/play/cg_damagePlums.mp4"
          />
          <figcaption>
            Each hit floats up with the exact damage you dealt.
          </figcaption>
        </figure>
        <PlatformOnly platform="flatscreen">
          <p>
            Toggle <strong>Setup → Game Options → Damage Numbers</strong>, or
            set <code>cg_damagePlums 1</code> in autoexec.
          </p>
        </PlatformOnly>
        <PlatformOnly platform={["pcvr", "quest"]}>
          <p>
            Set <code>cg_damagePlums 1</code> in autoexec.
          </p>
        </PlatformOnly>

        <h3 className="docs-play__feature-title">
          Blood Particles <code>cg_bloodParticles</code>
        </h3>
        <p>
          Particle blood with wall and floor splats, replacing the default
          sprite blood.
        </p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/cg_bloodParticles.webm"
            mp4="/assets/play/cg_bloodParticles.mp4"
          />
          <figcaption>
            Particle blood landing on walls and floor instead of the default
            sprite splatter.
          </figcaption>
        </figure>
        <PlatformOnly platform="flatscreen">
          <p>
            Toggle <strong>Setup → Game Options → Blood Particles</strong>, or
            set <code>cg_bloodParticles 1</code> in autoexec.
          </p>
        </PlatformOnly>
        <PlatformOnly platform={["pcvr", "quest"]}>
          <p>
            Set <code>cg_bloodParticles 1</code> in autoexec.
          </p>
        </PlatformOnly>

        <h3 className="docs-play__feature-title">
          Damage Effect <code>cg_damageEffect</code>
        </h3>
        <p>
          Directional red vignette overlay when taking damage — replaces the
          default screen-wide blood splatter with a subtler indicator that also
          tells you which direction the hit came from.
        </p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/cg_damageEffect.webm"
            mp4="/assets/play/cg_damageEffect.mp4"
          />
          <figcaption>
            Red flares from the side the hit came from, fading quickly so it
            doesn't obscure the screen.
          </figcaption>
        </figure>
        <PlatformOnly platform="flatscreen">
          <p>
            Toggle <strong>Setup → Game Options → Modern Damage Effect</strong>,
            or set <code>cg_damageEffect 1</code> in autoexec.
          </p>
        </PlatformOnly>
        <PlatformOnly platform={["pcvr", "quest"]}>
          <p>
            Toggle <strong>Setup → Game Options → Blood Spatter Effect</strong>,
            or set <code>cg_damageEffect 1</code> in autoexec.
          </p>
        </PlatformOnly>

        <h3 className="docs-play__feature-title">
          Improved stencil shadows <code>cg_shadows 2</code>
        </h3>
        <p>
          Real shadow volumes for player models, clipped against BSP geometry so
          they don't bleed through walls. The default <code>cg_shadows 1</code>{" "}
          draws blob shadows; <code>2</code> switches to stencil; <code>0</code>{" "}
          disables them. Set in autoexec.
        </p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/cg_shadows.webm"
            mp4="/assets/play/cg_shadows.mp4"
          />
          <figcaption>Stencil shadow cast by the green armor.</figcaption>
        </figure>
        <PlatformOnly platform={["pcvr", "quest"]}>
          <p>
            Heavier than blobs — Quest 3 hardware can handle it; older headsets
            often can't.
          </p>
        </PlatformOnly>

        <h3 className="docs-play__feature-title">
          Greyscale rendering <code>r_mapGreyScale</code>
        </h3>
        <p>
          A visibility aid: <code>r_mapGreyScale 1</code> desaturates the world
          geometry and lightmaps to greyscale while leaving entities (players,
          weapons, items, projectiles, explosions) in full color. Enemies and
          powerups visibly pop against the grey walls.
        </p>
        <figure className="docs-hud-figure">
          <ZoomableImage
            src="/assets/play/r_mapgreyscale.png"
            alt="A Trinity match rendered with r_mapGreyScale 1: the map textures are desaturated to grey while the player model, rocket explosion, lava, and on-screen powerup icons remain in full color."
          />
          <figcaption>
            <code>r_mapGreyScale 1</code> in action — map grey, entities and
            effects in color.
          </figcaption>
        </figure>
        <p>
          Latched: set it in <code>autoexec.cfg</code>, or run{" "}
          <code>vid_restart</code> after changing it. Negative values desaturate
          only the lightmaps for a subtler effect.
        </p>
        <p>
          The related <code>r_greyscale</code> turns the entire frame greyscale
          (entities included) — less useful for gameplay but handy for
          screenshots or aesthetic preference. Range <code>-1</code> to{" "}
          <code>1</code>; requires <code>r_fbo 1</code>. Both cvars come from
          Quake3e, the renderer Trinity's flatscreen engine is built on.
        </p>

        <h3 className="docs-play__feature-title">
          Hit Sounds <code>cg_hitSounds</code>
        </h3>
        <p>
          Damage-scaled audio feedback on successful hits — a different tone
          fires depending on how much damage you dealt. Set{" "}
          <code>cg_hitSounds 1</code> in autoexec for lower pitch on bigger
          hits, or <code>2</code> for higher pitch on bigger hits.
        </p>
        <p>The four tones, by damage dealt:</p>
        <ul className="docs-hitsounds">
          <li>
            <span className="docs-hitsounds__label">1–25</span>
            <HitSoundPreview src="/assets/play/hitsounds/hit25.wav" />
          </li>
          <li>
            <span className="docs-hitsounds__label">26–50</span>
            <HitSoundPreview src="/assets/play/hitsounds/hit50.wav" />
          </li>
          <li>
            <span className="docs-hitsounds__label">51–75</span>
            <HitSoundPreview src="/assets/play/hitsounds/hit75.wav" />
          </li>
          <li>
            <span className="docs-hitsounds__label">76+</span>
            <HitSoundPreview src="/assets/play/hitsounds/hit100.wav" />
          </li>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="forced-models">Forced enemy / team models</DocsH2>
        <p>
          On chaotic maps with stock skins everywhere, telling enemies from
          teammates from corpses takes a beat longer than it should. Trinity
          lets you force every enemy and every teammate to render as a model +
          skin you pick, with optional color overrides for the head, torso,
          legs, and rail.
        </p>
        <figure className="docs-hud-figure">
          <ZoomableImage
            src="/assets/play/cg_enemycolors.png"
            alt="A Trinity match with forced models active: two cyan Doom models in motion (forced enemy model + cg_enemyColors 333), a green Crash model in the background (forced team model + cg_teamColors 222), and two darkened bodies on the ground (cg_deadBodyDarken 1)."
          />
          <figcaption>
            Cyan Dooms = enemies, green Crash = teammate, darkened bodies on the
            ground = dead.
          </figcaption>
        </figure>
        <p>The config that produced the shot above:</p>
        <CopyableCommand>{`set cg_enemyModel "doom/pm"
set cg_enemyColors "333"
set cg_teamModel "crash/pm"
set cg_teamColors "222"
set cg_deadBodyDarken 1`}</CopyableCommand>
        <p>
          Skins suffixed with <code>/pm</code> (promode) or <code>/fb</code>{" "}
          (fullbright) accept the color overrides; stock skins ignore them. The
          dead-body darkening cvar (<code>cg_deadBodyDarken</code>) uses the
          same skin family.
        </p>
        <p>
          The color string is up to <strong>five characters</strong>: head,
          body, legs, <em>color1</em> (rail core / weapon glow), <em>color2</em>{" "}
          (rail spiral). Three characters (the common case) sets just the model
          tints; the enemy's own rail and weapon colors are left alone. Pass
          four or five characters if you also want to override their rail.
        </p>
        <p>
          The color numbers are the same ones the player-settings color slider
          uses to write <code>color1</code>. A <code>?</code> placeholder in any
          position becomes the player's team color — red, blue, or white (in
          FFA) — so <code>cg_enemyColors "????"</code> tints every enemy (and
          their rail) in their team's color, automatically.
        </p>
        <p>
          The digits don't match the chat <code>^N</code> codes most players
          know from typing colored names — a long-standing Quake 3 quirk.{" "}
          <Link to="/docs/reference#color-codes">Reference · Color codes</Link>{" "}
          has the side-by-side translation.
        </p>
        <p>
          This system has no in-game menu — drop the cvars into your{" "}
          <code>autoexec.cfg</code> with the values you want.{" "}
          <Link to="/docs/customize#player-models">
            Customize · Forced enemy / team models
          </Link>{" "}
          has the cvar reference.
        </p>
      </div>
    </>
  );
}
