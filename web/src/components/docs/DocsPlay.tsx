import { useState } from "react";
import { Link } from "react-router-dom";
import { DocsH2 } from "./DocsH2";
import { DocsModeTabs } from "./DocsModeTabs";
import { PlatformOnly } from "./PlatformOnly";
import { CopyableCommand } from "./CopyableCommand";
import { ZoomableImage } from "./ZoomableImage";
import { CalloutImage, CalloutLegend, type CalloutData } from "./CalloutImage";
import { HitSoundPreview } from "./HitSoundPreview";
import { LazyVideo } from "./LazyVideo";
import { MODE_PROFILES } from "../ServerCard";

// Each entry feeds both the CalloutImage overlay and the legend below it.
// Coordinates are percentages of the source image (the full-size shots in
// files-for-play-guide/): divide a pixel rect's x and width by the source
// width, its y and height by the source height, and multiply by 100.
// Source dims for hud-baseq3.png: 2560×1440.
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
        speaker icon overlaid on the portrait shows your VoIP state: a red X
        means you've muted yourself, and a colored speaker means you're
        transmitting on that channel. See{" "}
        <Link to="#voice-chat">Voice chat</Link> for the channel colors.
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
        Current armor with the armor icon, yellow at 50 here. Under CPMA rules (
        <code>g_mode 1</code>), the icon is green, yellow, or red for the
        armor&rsquo;s tier, and each tier has its own maximum and absorption
        rate. See <Link to="#modes">Game modes</Link>.
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
        you're leading). The yellow outline anchors to <em>your</em> score.
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
        Briefly names whatever you just picked up: armor here, and ammo, health,
        powerups, and weapons the same way.
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

// Source dims for hud-teamarena.png: 2560×1440.
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
        Rolling notify area at the top-left, where chat messages, kill feed
        entries, and server messages interleave. When a voice-chat macro is
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
        of the same kind.
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
        Same upper-right slot as vanilla Quake III. The <code>OT</code> prefix
        means the match is in overtime.
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
    body: <>The blue flag on its stand, which you&rsquo;re defending in CTF.</>,
  },
  {
    n: 5,
    xPct: 37.383,
    yPct: 24.722,
    wPct: 25.82,
    hPct: 9.028,
    title: "Center-print notification",
    body: <>Frag confirmations, flag pickups, and other game events.</>,
  },
  {
    n: 6,
    xPct: 42.813,
    yPct: 34.097,
    wPct: 3.242,
    hPct: 4.236,
    title: "Teammate indicator",
    body: (
      <>Downward-pointing yellow triangle that floats above teammates' heads.</>
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
        in the world: Guard, Ammo Regen, Scout, or Doubler (the red Doubler
        here). Once picked up, it stays with you until you die.
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
        The game picks a teammate to show here, usually the flag carrier in CTF
        and 1FCTF, with their name, current location, role icon, health, armor,
        and current weapon.
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
    body: (
      <>Active weapon icon and ammo count: the railgun with 35 slugs here.</>
    ),
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
        The small speaker icon overlaid on the portrait shows your VoIP state;
        the cyan speaker here means you&rsquo;re on the team channel. See{" "}
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
        A separate slot in the Team Arena layout, shown even at <code>0</code>.
        The baseq3 HUD hides armor until you pick some up.
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
        task icon (the upward green arrow is <em>attack</em>), plus your current
        map location. Set via the <code>task*</code> commands listed below, or
        from the in-game <strong>Team Orders</strong> menu.
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
        cards. The smaller number under each is the <em>game score</em>: Team
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
        jitter trace below.
      </>
    ),
  },
];

// Source dims for scoreboard-baseq3.png: 2560×1440.
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
        behind the badge for VR clients. VoIP speaker icons appear at the
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
    title: "Timer + mode icon",
    body: (
      <>
        The same upper-right stack as the in-match HUD. The mode icon next to
        the timer reports the active <code>g_mode</code> profile, vanilla Quake
        III in this shot.
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
        Countdown for an active timed powerup: Quad Damage with 15 seconds
        remaining here. Appears whenever you're holding a timed powerup.
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
        jitter trace below.
      </>
    ),
  },
];

// Source dims for scoreboard-teamarena.png: 2560×1440.
const SCOREBOARD_TEAMARENA_CALLOUTS: CalloutData[] = [
  {
    n: 1,
    xPct: 42.813,
    yPct: 16.875,
    wPct: 16.758,
    hPct: 5.486,
    title: "Team scores headline",
    body: null,
  },
  {
    n: 2,
    xPct: 88.398,
    yPct: 0,
    wPct: 11.602,
    hPct: 8.472,
    title: "Timer + mode icon",
    body: (
      <>
        Same upper-right stack as the in-match HUD. The <code>OT</code> prefix
        means the match is in overtime. The mode icon reports the active{" "}
        <code>g_mode</code> profile, vanilla Quake III in this shot.
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
        The role each player has declared via <code>task*</code>: attacking
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
        client).
      </>
    ),
  },
  {
    n: 5,
    xPct: 54.336,
    yPct: 29.653,
    wPct: 5.43,
    hPct: 6.667,
    title: "Leader + VoIP",
    body: (
      <>
        One player per team can be designated team leader (a voteable status).
        The leader can issue orders that bots respect. The VoIP speaker icon
        (cyan here, the team channel) shows who&rsquo;s speaking on which
        channel while the scoreboard is up.
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
        impressives, captures. These are the medal categories.
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
        jitter trace below.
      </>
    ),
  },
];

// Source dims for demo-player.png, captured from the web demo player: 1467×991.
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
        The same scoreboard cell that shows bot difficulty for AI players shows
        a <code>[VR]</code> badge for VR clients.
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
        The top right shows whether the followed player is in VR, plus a list of
        recent VoIP speakers (channel-colored). Inactive entries fade out, so
        the list only shows people who are speaking or just spoke.
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

// Platform differences go in paired PlatformOnly blocks, one per platform,
// so a reader sees only their client's variant. Cvar lists belong on
// /docs/reference.

// Team Arena role declarations: HUD icon (extracted from missionpack
// ui/assets/statusbar/*.tga, hence the engine asset names) + task name.
const TEAMARENA_TASKS = [
  { icon: "assault", label: "Offense" },
  { icon: "defend", label: "Defense" },
  { icon: "patrol", label: "Patrol" },
  { icon: "camp", label: "Camp" },
  { icon: "follow", label: "Follow" },
  { icon: "retrieve", label: "Retrieve" },
  { icon: "escort", label: "Escort" },
] as const;

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
          What your Trinity client adds to vanilla Quake III: visual feedback,
          voice chat, demo recording and playback,{" "}
          <PlatformOnly platform="flatscreen">
            and persistent accounts.
          </PlatformOnly>
          <PlatformOnly platform="vr">
            persistent accounts, and full motion tracking.
          </PlatformOnly>{" "}
          Trinity still connects to standard Quake III servers, so installing it
          doesn&rsquo;t limit which servers you can join.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="q3-basics">Quake III basics</DocsH2>
        <details className="docs-collapsible">
          <summary>
            For new players: goal, game types, weapons, and items. Skip it if
            you&rsquo;ve played Quake III before.
          </summary>

          <h3 className="docs-play__feature-title">Goal &amp; format</h3>
          <p>
            Most matches share a format: a time limit, a score limit, and
            whichever comes first ends the match. The player or team in the lead
            at that moment wins. Servers usually advertise both, such as{" "}
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
              <strong>Free For All:</strong> everyone against everyone.
            </li>
            <li>
              <strong>Tournament:</strong> 1v1.
            </li>
            <li>
              <strong>Team Deathmatch:</strong> team against team, scored by
              frags.
            </li>
            <li>
              <strong>Capture the Flag:</strong> grab their flag, return yours,
              score.
            </li>
            <li>
              <strong>One-Flag CTF</strong> <em>(Team Arena)</em>: one neutral
              flag, delivered to the enemy base.
            </li>
            <li>
              <strong>Overload</strong> <em>(Team Arena)</em>: destroy the enemy
              obelisk.
            </li>
            <li>
              <strong>Harvester</strong> <em>(Team Arena)</em>: collect skulls
              from kills and deposit them at the enemy receptacle.
            </li>
          </ul>

          <h3 className="docs-play__feature-title">Weapons</h3>
          <p>Quake III has nine weapons; Team Arena adds three more.</p>
          <PlatformOnly platform="vr">
            <p>
              Switch weapons with the weapon wheel: hold your weapon
              hand&rsquo;s grip to bring it up, point at the slot you want, and
              release to select. The gauntlet joins the wheel with{" "}
              <strong>
                Setup → VR Options → Controls → Gauntlet On Weapon Wheel
              </strong>
              .
            </p>
          </PlatformOnly>
          <ul>
            <li>
              <strong>Gauntlet:</strong> chainsaw melee. Killing with it
              triggers a humiliation indicator in the kill feed.
            </li>
            <li>
              <strong>Machinegun:</strong> accurate hitscan, light damage per
              round.
            </li>
            <li>
              <strong>Shotgun:</strong> close-range pellet spread.
            </li>
            <li>
              <strong>Grenade Launcher:</strong> bouncing, arcing projectiles.
            </li>
            <li>
              <strong>Rocket Launcher:</strong> splash damage, and the fuel for
              rocket jumps.
            </li>
            <li>
              <strong>Lightning Gun:</strong> short-range continuous beam.
            </li>
            <li>
              <strong>Railgun:</strong> slow-firing instant-hit sniper.
            </li>
            <li>
              <strong>Plasma Gun:</strong> rapid-fire energy projectiles.
            </li>
            <li>
              <strong>BFG10K:</strong> a superweapon, rarely placed on maps.
            </li>
            <li>
              <strong>Nailgun</strong> <em>(Team Arena)</em>: massive damage at
              close range, but the spread and projectile speed make it tricky at
              a distance.
            </li>
            <li>
              <strong>Prox Launcher</strong> <em>(Team Arena)</em>: lays
              proximity mines that detonate when an enemy gets close, good for
              denying an area.
            </li>
            <li>
              <strong>Chaingun</strong> <em>(Team Arena)</em>: belt-fed
              rapid-fire bullet weapon for sustained damage.
            </li>
            <li>
              <strong>Grapple</strong> <em>(Trinity, server-enabled)</em>: a
              tenth weapon some servers hand out at spawn. See{" "}
              <Link to="/docs/play#grapple">Grapple</Link> below.
            </li>
          </ul>

          <h3 className="docs-play__feature-title">Items</h3>
          <p>
            Pickups respawn on timers that depend on the game type and the
            server's <code>g_mode</code> profile.
          </p>
          <p>
            Trinity predicts your pickups, so an item vanishes and your health,
            armor, or ammo changes the moment you touch it.{" "}
            <code>cg_predictItems</code> controls this and is on by default.
          </p>
          <ul>
            <li>
              <strong>Armor:</strong> in vanilla Quake III, Shard (+5), Yellow
              (+50), and Heavy/Red (+100), up to 200. CPMA rules replace this
              with a tiered system that adds a lower-tier green armor, with
              absorption that depends on the tier; see <code>g_mode</code> in{" "}
              <Link to="/docs/reference#server-cvars">Server CVars</Link>.
            </li>
            <li>
              <strong>Health:</strong> 5, 25, 50, and Mega Health (+100,
              stacking past your normal cap).
            </li>
            <li>
              <strong>Powerups:</strong> Quad Damage, Battle Suit, Speed
              (Haste), Regeneration, Invisibility (all 30 seconds); Flight (60
              seconds).
            </li>
            <li>
              <strong>Persistent powerups</strong> <em>(Team Arena)</em>: Guard,
              Ammo Regen, Scout, and Doubler. These have no countdown; once
              picked up, they stay with you until you die.
            </li>
            <li>
              <strong>Holdables</strong> (use when ready): Personal Teleporter
              and Medkit. Team Arena adds Kamikaze and Invulnerability.
            </li>
          </ul>
        </details>
      </div>

      <div className="about-section">
        <DocsH2 id="accounts">Trinity accounts</DocsH2>
        <p>
          Trinity records the matches you play on participating servers. Without
          an account, those records attach to the per-install identity Quake III
          generates (your <code>qkey</code>), and each new install counts as a
          new player to the hub.
        </p>
        <p>
          A Trinity account ties every install you sign in on to one record,
          with the same name and stats on the leaderboards. Setup is one in-game
          command and a quick visit to this site; the{" "}
          <Link to="/docs/account">Account</Link> page walks through it.
        </p>
        <p>
          To find Trinity-collected servers in-game, open{" "}
          <strong>Multiplayer</strong> from the main menu, then click the{" "}
          <em>Servers:</em> selector at the top until it reads{" "}
          <strong>Trinity</strong>, which filters the list to servers registered
          with the hub. The <Link to="/servers">Servers</Link> page lists the
          same servers with live status.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="voice-chat">Voice chat</DocsH2>
        <p>
          Voice chat works on servers that enable it and is recorded into
          TrinityVision demos, so you hear the chatter on playback. There are
          three targets you cycle through, plus a fourth you set by hand:
        </p>
        <ul>
          <li>
            <strong>spatial:</strong> everyone nearby hears you, quieter with
            distance.
          </li>
          <li>
            <strong>team:</strong> your team only, in team modes.
          </li>
          <li>
            <strong>all:</strong> every player on the server hears you.
          </li>
          <li>
            <strong>direct:</strong> a comma-separated list of player IDs, or
            the keywords <code>attacker</code> or <code>crosshair</code>. Set it
            on <code>cl_voipSendTarget</code> directly; the cycle skips it.
          </li>
        </ul>
        <p>
          The <code>voiptarget</code> command cycles through the first three in
          order: <em>spatial → team (in team modes) → all → spatial</em>. Bind a
          one-shot <em>set cl_voipSendTarget …</em> if you want a hot key for
          whispering to a particular teammate.
        </p>

        <h3 className="docs-play__feature-title">Spotting who's talking</h3>
        <p>
          Channel-colored speaker icons show voice activity in three places
          during play:
        </p>
        <ul>
          <li>
            <strong>Player portrait icon:</strong> a speaker icon on your own
            portrait shows when your mic is transmitting.
          </li>
          <li>
            <strong>Upper-right speakers list:</strong>{" "}
            <code>cg_drawVoipSpeakers 1</code> stacks the names of the players
            speaking right now in the upper right of the HUD.
          </li>
          <li>
            <strong>Over-head indicators:</strong> speaker icons appear above
            speaking players&rsquo; models.
          </li>
        </ul>
        <p>
          The number of arcs on each icon shows the speaker&rsquo;s peak volume,
          and at 90% or more the icon brightens.
        </p>
        <VoipLevelStrip />

        <h3 className="docs-play__feature-title">Activating voice chat</h3>
        <p>
          Voice chat settings are in <strong>Setup → System → Sound</strong>:
          turn <em>VOIP</em> on or off, choose an <em>Activation</em> mode, set
          the voice-activity <em>Threshold</em>, and adjust <em>VOIP Volume</em>{" "}
          and <em>Transmit Game Volume</em>, which lowers the game&rsquo;s sound
          while you talk. The two activation modes:
        </p>
        <div className="docs-table-scroll">
          <table className="docs-color-table">
            <thead>
              <tr>
                <th></th>
                <th>Push to talk</th>
                <th>When speaking</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <th scope="row">When mic transmits</th>
                <td>only while you hold a bound input</td>
                <td>whenever the mic picks up sound above the Threshold</td>
              </tr>
              <tr>
                <th scope="row">Bind to</th>
                <td>
                  <strong>VoIP PTT / Toggle</strong>: hold to talk
                </td>
                <td>
                  <strong>VoIP PTT / Toggle</strong>: tap to mute or unmute
                </td>
              </tr>
              <tr>
                <th scope="row">Best for</th>
                <td>noisy rooms, controlled chatter</td>
                <td>quiet rooms, good noise-canceling mic</td>
              </tr>
            </tbody>
          </table>
        </div>

        <PlatformOnly platform="flatscreen">
          <p>
            Bind <strong>VoIP PTT / Toggle</strong> and{" "}
            <strong>Voice Target</strong> under{" "}
            <strong>Setup → Controls</strong>, next to the chat keys. Neither
            has a default key.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="vr">
          <p>
            It's bound already: hold the <strong>turn stick click</strong> to
            talk, or with voice activity, click it to mute or unmute. Hold{" "}
            <strong>Alt</strong> while clicking it to cycle the channel. Alt is
            the thumbrest on Touch controllers and a bumper on the Steam Frame,
            and it gives every button a second action. Change either in{" "}
            <strong>Setup → VR Options → Bindings</strong>.
          </p>
        </PlatformOnly>

        <p>
          Channel mutes, the on-HUD speaker list, and the engine mic meter are
          set only in <code>autoexec.cfg</code>;{" "}
          <Link to="/docs/reference#player-cvars">Player CVars</Link> lists the
          full set.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="vr-tracking">1:1 head and weapon tracking</DocsH2>
        <p>
          Trinity tracks VR players&rsquo; head movement and weapon aim 1:1, so
          their models move the way the players do. It&rsquo;s subtle, and every
          player sees it, flatscreen and VR alike, in live play and demo
          playback. It&rsquo;s always on.
        </p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/vr_tracking.webm"
            mp4="/assets/play/vr_tracking.mp4"
          />
          <figcaption>
            A VR player&rsquo;s head pose and weapon aim, tracked in real time,
            as every other player in the match sees them.
          </figcaption>
        </figure>
      </div>

      <PlatformOnly platform="vr">
        <div className="about-section">
          <DocsH2 id="vr" platforms={["vr"]}>
            VR-specific features
          </DocsH2>

          <h3 className="docs-play__feature-title">
            Two-handed weapon grip <code>vr_twoHandedWeapons</code>
          </h3>
          <p>
            Hold the secondary grip to stabilize the weapon. Pick a mode in{" "}
            <strong>Setup → VR Options → Controls → Two-Handed Weapons</strong>{" "}
            or with <code>vr_twoHandedWeapons</code> in autoexec: <code>0</code>{" "}
            turns it off, <code>1</code> is Basic (the default), and{" "}
            <code>2</code> is VR Gun Stock.
          </p>
          <ul>
            <li>
              <strong>Basic:</strong> the weapon aims along a line from your
              primary controller toward your secondary controller.
            </li>
            <li>
              <strong>VR Gun Stock:</strong> the weapon anchors near your face,
              like sighting along a stock.
            </li>
          </ul>

          <h3 className="docs-play__feature-title">
            In-game weapon position adjustment
          </h3>
          <p>
            Trinity lets you tune how each weapon sits in your hand without
            leaving the game.
          </p>
          <p>
            Adjustment mode is the <strong>Weapon adjust</strong> action in{" "}
            <strong>Setup → VR Options → Bindings</strong> (console command{" "}
            <code>weapon_adjust</code>). It has no default binding.
          </p>
          <p>
            Adjustment mode shows a parameter strip at the top of your view with
            seven values you can tune for the current weapon: <em>scale</em>,{" "}
            <em>right</em>, <em>up</em>, <em>fwd</em>, <em>pitch</em>,{" "}
            <em>yaw</em>, and <em>roll</em>.
          </p>
          <figure className="docs-hud-figure">
            <ZoomableImage
              src="/assets/play/weapon-adjust.jpg"
              alt="In-VR view of weapon adjustment mode: the parameter strip across the top of the screen lists scale, right, up, fwd, pitch, yaw, and roll, with the currently-selected parameter highlighted."
            />
            <figcaption>
              The parameter strip across the top of your view in adjustment
              mode.
            </figcaption>
          </figure>
          <ul>
            <li>
              <strong>Secondary thumbstick left / right:</strong> cycles which
              parameter is selected.
            </li>
            <li>
              <strong>Primary thumbstick up / down:</strong> adjusts the
              selected value; small deflections make fine adjustments.
            </li>
            <li>
              <strong>A button:</strong> accept and exit adjustment mode.
            </li>
            <li>
              <strong>B button:</strong> reset to default. <em>Tap</em> to reset
              the selected parameter; <em>hold for two seconds</em> to reset
              every parameter on the current weapon. A bar fills while you hold,
              and the controller buzzes when the full reset fires.
            </li>
          </ul>
          <p>
            The strip's footer names the buttons you have bound; A and B are the
            defaults, changeable in the Adjust rows of{" "}
            <strong>Setup → VR Options → Bindings</strong>.
          </p>
          <p>
            Switch weapons mid-adjustment to tune another one, and the strip
            follows. Changes save to the per-weapon cvars (
            <code>vr_weapon_adjustment_N</code>, one per weapon slot) as you
            make them, so they persist across sessions. Adjustment mode exits on
            its own if you die, open the virtual screen, scope a weapon, or
            start spectating.
          </p>

          <h3 className="docs-play__feature-title">Comfort options</h3>
          <p>
            Comfort vignette, height adjust, haptic intensity, and the
            single-player 6DoF toggle are in{" "}
            <strong>Setup → VR Options → Comfort</strong>. Snap turn is under{" "}
            <strong>Setup → VR Options → Controls</strong>, and HUD depth,
            scale, and vertical position under{" "}
            <strong>Setup → VR Options → HUD &amp; Display</strong>.{" "}
            <Link to="/docs/reference#vr-cvars">VR CVars</Link> lists the cvar
            names for setting them in <code>autoexec.cfg</code>.
          </p>

          <h3 className="docs-play__feature-title">
            Refresh rate and supersampling
          </h3>
          <p>
            <strong>Refresh Rate</strong> and <strong>Supersampling</strong> are
            under <strong>Setup → VR Options → HUD &amp; Display</strong>. The
            refresh-rate row lists only the rates your headset's runtime
            reports, lowest first, and takes effect as soon as you pick one.
            Higher refresh and supersampling above 1.0 both cost frame budget.
          </p>

          <h3 className="docs-play__feature-title">Foveated rendering</h3>
          <p>
            Trinity renders the edges of your view in less detail than the
            center, which frees up GPU time for a higher refresh rate or heavier
            settings. It's on by default. The controls are under{" "}
            <strong>Setup → VR Options → HUD &amp; Display</strong>:
          </p>
          <ul>
            <li>
              <strong>Foveated Rendering:</strong> Off, Fixed, or Eye-Tracked.
              Fixed keeps the sharp region at the center of each eye's view;
              Eye-Tracked moves it with your gaze. Eye-Tracked is only offered
              on headsets with eye tracking.
            </li>
            <li>
              <strong>Foveation Strength:</strong> Low, Medium, or High. Higher
              saves more GPU time and is more visible at the edges. Medium by
              default.
            </li>
          </ul>
          <p>
            If neither row is there, your headset doesn't offer foveated
            rendering and Trinity draws full detail everywhere. If your config
            asks for a mode your headset can't do, the menu shows what's in
            force.
          </p>
          <p>
            On a Quest or PICO with eye tracking, Trinity asks for the
            eye-tracking permission the first time it runs.
          </p>
          <p>
            On PC VR through SteamVR's Steam Link, Eye-Tracked needs the
            headset's eye data shared with the PC: if the sharp region doesn't
            follow your gaze, turn on{" "}
            <strong>Share eye tracking data to other apps on this PC</strong> in
            SteamVR's Steam Link settings.
          </p>

          <h3 className="docs-play__feature-title">VR controls</h3>
          <p>
            Direction mode (head or off-hand controller), snap turn angle,
            handedness, weapon scope (railgun zoom), trigger sensitivity, and
            the thumbstick swap are all in{" "}
            <strong>Setup → VR Options → Controls</strong>. Buttons are bound in{" "}
            <strong>Setup → VR Options → Bindings</strong>, with a Button and an
            Alt + Button column per action and your controller&rsquo;s own
            button glyphs; see <Link to="/docs/customize#vr">Customize</Link>.
          </p>

          <h3 className="docs-play__feature-title">VR keyboard</h3>
          <p>
            When you need to type chat messages, console commands, or names,
            Trinity opens a keyboard on the virtual screen, laid out like a 65%
            PC keyboard: the main block, Home / End / Page Up / Page Down, and
            arrow keys. Either hand types: point at a key, where a pool of light
            marks it, and pull the trigger; hold the trigger to repeat. Close it
            with Escape, the Menu button, or a click outside the keyboard.
          </p>
          <p>
            It opens when you reach a text field with the stick, a click, or
            Enter, and sits above or below the field you're editing so the text
            stays visible.
          </p>
          <p>
            While it's open, the controller buttons bound in the{" "}
            <strong>VKB</strong> tab of{" "}
            <strong>Setup → VR Options → Bindings</strong> type Space,
            Backspace, Tab, and Enter; Backspace repeats while held. Each bound
            button's glyph shows in the corner of its key.
          </p>

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
            a VR client. Playing Quake III in VR asks more of the player than
            flatscreen does, and the icon tells flatscreen opponents who
            they&rsquo;re up against. The same icon shows up when you follow a
            VR player as a spectator.
          </p>

          <h3 className="docs-play__feature-title">
            Pure servers and VR-aware game code
          </h3>
          <p>
            A <code>sv_pure 1</code> server running a VR-aware mod accepts you
            in VR. The mod has to be <strong>built for VR</strong>:
            Trinity&rsquo;s mod is, and most other mods aren&rsquo;t. On a
            non-pure baseq3 or Team Arena server, Trinity uses its own VR game
            code. Anywhere else, a mod without VR support sends you to the main
            menu, still in VR, with a message naming the pak. On a PC you can
            switch to flatscreen to play it.
          </p>
        </div>
      </PlatformOnly>

      <div className="about-section">
        <DocsH2 id="hud">HUD</DocsH2>
        <p>
          The layout depends on whether the server runs vanilla Quake III or
          Team Arena, the expansion, which shows more objective and team
          information.
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
              directional damage indicator, which you&rsquo;ll see constantly in
              matches.
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
              declare what they're doing for the team from the in-game{" "}
              <strong>Team Orders</strong> menu. The declaration shows up on
              teammates' HUDs as an icon alongside the player's location:
            </p>
            <ul className="docs-tasklist">
              {TEAMARENA_TASKS.map((task) => (
                <li key={task.icon}>
                  <img
                    src={`/assets/play/tasks/${task.icon}.png`}
                    alt=""
                    className="docs-tasklist__icon"
                  />
                  <strong>{task.label}</strong>
                </li>
              ))}
            </ul>
          </DocsModeTabs.Panel>
        </DocsModeTabs>
      </div>

      <div className="about-section">
        <DocsH2 id="scoreboard">Scoreboard</DocsH2>
        <PlatformOnly platform="vr">
          <p>Click the move stick to pull it up.</p>
        </PlatformOnly>
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
        <DocsH2 id="modes">Game modes</DocsH2>
        <p>
          Trinity has four game modes, each pairing movement physics with combat
          rules: Quake III Arena (<code>g_mode 0</code>), CPMA (<code>1</code>),
          Quake Live (<code>2</code>), and Quake Live Turbo (<code>3</code>).
          Online, the server picks. When you host a server or play single-player
          against bots, choose one with the <strong>Mode</strong> selector in
          the create-server and single-player menus, or set <code>g_mode</code>{" "}
          at the console and run <code>map_restart</code>.
        </p>
        <p>
          The server browser shows each server's mode as an icon to the left of
          its name; the same icons appear on scoreboards:
        </p>
        <div className="docs-modes">
          {Object.values(MODE_PROFILES).map((m) => (
            <div key={m.icon} className="docs-mode-item">
              <img src={m.icon} alt={m.label} className="docs-mode-icon" />
              <div className="docs-mode-info">
                <span className="docs-mode-name">{m.label}</span>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="grapple">Grapple</DocsH2>
        <p>
          Servers can hand every player a grapple: a tenth weapon in your
          loadout from the moment you spawn that fires a chisel-clawed anchor
          pad on a tether and pulls you to wherever it bites.
        </p>
        <p>
          Set <strong>Grapple</strong> in the create server or single player
          menu, or using <code>g_grapple 1</code> at the console or for a
          dedicated server. It will be granted to players on their next spawn.
        </p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/grapple.webm"
            mp4="/assets/play/grapple.mp4"
          />
          <figcaption>
            The pad stuck to the underside of a mover on q3dm19, a map that is
            all moving platforms and long drops into the void. Expect the
            grapple to save your life here more than once.
          </figcaption>
        </figure>
        <h3 className="docs-play__feature-title">Using it</h3>
        <PlatformOnly platform="flatscreen">
          <p>
            It's on <code>0</code>, the key after the BFG (
            <strong>Setup → Controls → Shoot → Grapple</strong>), and weapon
            next/previous cycles to it as well.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="vr">
          <p>It joins the weapon wheel whenever the server hands it out.</p>
        </PlatformOnly>
        <ul>
          <li>
            <strong>Hold fire to throw.</strong> When the pad bites a wall, a
            ceiling, or another player, the tether pulls you toward it. Keep
            holding to stay attached.
          </li>
          <li>
            <strong>Let go of fire (or switch weapons) to release.</strong>
          </li>
          <li>
            <strong>An enemy with your claws in them takes damage</strong> the
            whole time they stay attached (servers can tune that, or turn it
            off). Teammates are safe: a pad that touches one simply drops.
          </li>
          <li>
            <strong>It lets go on its own</strong> when either you or your
            target dies or teleports.
          </li>
          <li>
            <strong>Pads ride movers.</strong> Reach a pad stuck to a moving
            platform, door, or rotator and you ride along, turning with it if it
            rotates, and can climb over its edge.
          </li>
        </ul>
        <p>Bots use it too, to get around and to fight.</p>
      </div>

      <div className="about-section">
        <DocsH2 id="clans">Team Arena clans</DocsH2>
        <p>
          On Team Arena servers, red and blue are clans: Crusaders, Intruders,
          Pagans, Stroggs, The Fallen, and a sixth added specially for fans of
          VR ports, who bring an exceptionally beefy challenge. Bot teammates
          come from the named five-member roster of the clan they fight for,
          with the occasional free agent filling out a squad.
        </p>
        <p>
          Clans play winner-stays: when a match ends, the losing side's clan
          rotates out, banner and bots alike, and the next clan steps in to take
          up the fight. So if your team's name and faces changed between maps,
          you didn't imagine it: you're carrying a new clan's colors now.
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
            <code>cg_followMode 1</code> in autoexec; first-person (
            <code>0</code>) is the default. A third mode, <em>free-fly</em> (
            <code>cg_followMode 2</code>), works only during TV demo playback,
            so the menu leaves it out.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="vr">
          <p>
            The same <strong>Follow Camera</strong> setting applies in VR. While
            following, the turn stick orbits the camera, snapping if you use
            snap turn; the buttons that switch views, recenter, and free the
            camera are in the Follow rows of{" "}
            <strong>Setup → VR Options → Bindings</strong>.
          </p>
        </PlatformOnly>

        <h3 className="docs-play__feature-title">Scrub and pause TV demos</h3>
        <p>
          During TV demo playback, hold a key (or controller button) bound to{" "}
          <code>+tv_scrub</code> to scrub forward and backward through the
          timeline. Bind <code>demopause</code> to freeze a moment in time. Both
          are bind targets; set them up in your <code>autoexec.cfg</code>.
        </p>

        <h3 className="docs-play__feature-title">
          Watch live matches in the browser
        </h3>
        <p>
          You can watch any in-progress match on a participating server live in
          your browser, and you don&rsquo;t need an account. Wherever you see a
          server card (the <Link to="/">landing page</Link> or{" "}
          <Link to="/servers">Servers</Link>), a pulsing <strong>LIVE</strong>{" "}
          pill means the broadcast is on. Click it and the viewer opens
          mid-match, already following whoever is leading. From there you can
          follow any player or fly the free camera. The feed runs slightly
          behind the real game; hover over the pill for the current delay. The
          small number beside it is how many people are watching.
        </p>
        <p>
          The arena knows when it has an audience. When the first viewer
          arrives, players in the server hear{" "}
          <em>&ldquo;The Vadrigar watch.&rdquo;</em> and an eye with the viewer
          count appears on their HUDs. You&rsquo;re not invisible up there. Make
          them feel observed.
        </p>
        <p>
          Live viewing has no timeline to scrub, but every match is also
          recorded.
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
            <strong>In the browser:</strong> open any match from{" "}
            <Link to="/matches">Matches</Link> and the hub plays it back. The
            link works for anyone you share it with.
          </li>
          <li>
            <strong>In your local game client:</strong> download the{" "}
            <code>.tvd</code> file from the match page, or accept the post-match
            download prompt, which saves it to your demos folder. Local playback
            gives you the full <code>+tv_scrub</code> and <code>demopause</code>{" "}
            controls described above.
          </li>
        </ul>

        <figure className="docs-hud-figure">
          <CalloutImage
            src="/assets/play/demo-player.png"
            alt="Trinity web demo player showing scoreboard with VR badges in the handicap column, recent VoIP speakers in the upper right, and the playback timeline scrubber at the bottom."
            callouts={DEMO_PLAYER_CALLOUTS}
          />
          <figcaption>
            Watching a Trinity match in the browser: the same in-game
            scoreboard, plus the playback timeline at the bottom.
          </figcaption>
        </figure>
        <CalloutLegend callouts={DEMO_PLAYER_CALLOUTS} />

        <p>
          Set the post-match download prompt under{" "}
          <strong>Setup → Game Options → TVD Download</strong> (
          <code>cl_tvDownload</code>: Off / Decline / Accept).
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
        </figure>
        <p>
          On by default. Turn them off under{" "}
          <strong>Setup → Game Options → Damage Numbers</strong>, or with{" "}
          <code>cg_damagePlums 0</code>.
        </p>
        <p>
          <code>cg_damagePlumScale</code> multiplies their size (default{" "}
          <code>1.0</code>).
        </p>

        <h3 className="docs-play__feature-title">
          Modern Blood <code>com_blood 2</code>
        </h3>
        <p>
          Blood that scales with damage. Bigger hits throw more blood and mist
          at the wound, gibs spray and splatter off nearby walls, and the mess
          gets painted onto the floors, walls, and ceilings around the kill.
          Gibs fly in the direction of the killing blow and trail blood as they
          go: a rocket launches them across the room, while a gauntlet barely
          nudges them.
        </p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/modern_blood.webm"
            mp4="/assets/play/modern_blood.mp4"
          />
          <figcaption>
            Modern blood reacting to each hit: spray, lingering trails, and
            marks pooling on nearby surfaces. (Yeah, quad rockets are a little
            intense 😂)
          </figcaption>
        </figure>
        <p>
          Pick <strong>Setup → Game Options → Blood</strong> (Off / Classic /
          Modern), or set <code>com_blood</code> in autoexec.
        </p>
        <p>
          <strong>Setup → Game Options → Blood Density</strong>, under Blood,
          sets how much modern blood is drawn: Light, Medium, or Heavy (
          <code>cg_bloodDensity</code> <code>0</code> / <code>1</code> /{" "}
          <code>2</code>). It&rsquo;s grayed out unless Blood is Modern.{" "}
          <PlatformOnly platform="flatscreen">
            It defaults to Heavy.
          </PlatformOnly>
          <PlatformOnly platform="vr">
            It defaults to Heavy on a PC and Light on Steam Frame, Quest, and
            PICO.
          </PlatformOnly>
        </p>

        <h3 className="docs-play__feature-title">
          Modern Damage Effect <code>cg_damageEffect</code>
        </h3>
        <p>
          A red vignette on the side of the screen the hit came from when you
          take damage. It&rsquo;s subtler than the classic screen-wide blood
          splatter, and it tells you where the hit came from.
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
        <p>
          On by default. <strong>Setup → Game Options → Damage Effect</strong>{" "}
          switches back to Classic, as does <code>cg_damageEffect 0</code>.
        </p>

        <h3 className="docs-play__feature-title">
          Improved stencil shadows <code>cg_shadows 2</code>
        </h3>
        <p>
          Real shadow volumes for player models, clipped so they stay on the
          near side of walls. Pick it under{" "}
          <strong>Setup → Graphics → Shadows</strong> (<em>None</em>,{" "}
          <em>Low</em> for the default blob shadow, or <em>High</em> for
          stencil), or set <code>cg_shadows</code> to <code>0</code>,{" "}
          <code>1</code>, or <code>2</code> in autoexec.
        </p>
        <figure className="docs-hud-figure">
          <LazyVideo
            webm="/assets/play/cg_shadows.webm"
            mp4="/assets/play/cg_shadows.mp4"
          />
          <figcaption>Stencil shadow cast by the green armor.</figcaption>
        </figure>
        <p>
          Four cvars tune stencil shadows. <code>r_shadowDistance</code>{" "}
          (default <code>256</code> units) is how far away shadows are drawn.{" "}
          <code>r_shadowClip</code> (on by default),{" "}
          <code>r_shadowClipPenetration</code> (default <code>4</code>), and{" "}
          <code>r_shadowClipExtension</code> (default <code>16</code>) keep
          shadows off the far side of walls.{" "}
          <Link to="/docs/reference#player-cvars">Player CVars</Link> has the
          details.
        </p>
        <PlatformOnly platform="vr">
          <p>
            Stencil shadows cost more than blob shadows. A Quest 3 handles them;
            older standalone headsets may struggle.
          </p>
        </PlatformOnly>

        <h3 className="docs-play__feature-title">
          Light coronas <code>r_flares</code>
        </h3>
        <p>
          Lamps, torches, and the sun bloom into a soft corona when they come
          into view, and fade as you turn away or move behind cover. It's on by
          default; <code>r_flares 0</code> turns it off, and{" "}
          <code>r_flareSize</code> sets how wide the glow spreads, from{" "}
          <code>1</code> to <code>40</code> (the default).
        </p>
        <h3 className="docs-play__feature-title">
          Grayscale rendering <code>r_mapGreyScale</code>
        </h3>
        <p>
          A visibility aid: <code>r_mapGreyScale 1</code> desaturates the world
          geometry and lightmaps to grayscale and leaves entities (players,
          weapons, items, projectiles, explosions) in full color, so enemies and
          powerups stand out against the gray walls.
        </p>
        <figure className="docs-hud-figure">
          <ZoomableImage
            src="/assets/play/r_mapgreyscale.png"
            alt="A Trinity match rendered with r_mapGreyScale 1: the map textures are desaturated to gray while the player model, rocket explosion, lava, and on-screen powerup icons remain in full color."
          />
        </figure>
        <p>
          The range is <code>-1</code> to <code>1</code>, default <code>0</code>
          ; negative values desaturate only the lightmaps for a subtler effect.
          Set it in <code>autoexec.cfg</code>, or run <code>vid_restart</code>{" "}
          after changing it.
        </p>
        <p>
          The related <code>r_greyscale</code> turns the entire frame grayscale,
          entities included. Its range is also <code>-1</code> to <code>1</code>
          , and it requires <code>r_fbo 1</code>. Both cvars come from Quake3e,
          which Trinity is built on.
        </p>

        <PlatformOnly platform="flatscreen">
          <h3 className="docs-play__feature-title">
            True HDR output <code>r_hdrDisplay</code>
          </h3>
          <p>
            On an HDR display, Trinity can render in HDR, with brighter
            highlights on lights, explosions, plasma, and sky. It needs the
            Vulkan renderer and is off by default. Turn it on under{" "}
            <strong>Setup → Display → HDR Display</strong>, then calibrate it
            under <strong>Setup → Display → HDR Calibration</strong> (a Mac
            skips this step);{" "}
            <Link to="/docs/customize#display-output">Customize</Link> walks
            through it.
          </p>
          <p>
            A normal screenshot can&rsquo;t capture HDR, so you&rsquo;ll have to
            see it on your own display.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="vr">
          <h3 className="docs-play__feature-title">
            HDR on the desktop mirror <code>r_hdrDisplay</code>
          </h3>
          <p>
            On an HDR display, Trinity can render the desktop mirror window (
            <strong>Setup → VR Options → Desktop Mirror</strong>) in HDR, with
            brighter highlights. The headset itself renders standard range.
            Quest and PICO have no desktop mirror, and the Steam Frame starts
            with it off. HDR needs the Vulkan renderer and is off by default.
            Turn it on under <strong>Setup → Display → HDR Display</strong>,
            then calibrate under{" "}
            <strong>Setup → Display → HDR Calibration</strong>;{" "}
            <Link to="/docs/customize#display-output">Customize</Link> walks
            through it.
          </p>
          <p>A normal screenshot can&rsquo;t capture HDR.</p>
        </PlatformOnly>

        <h3 className="docs-play__feature-title">
          Hit Sounds <code>cg_hitSounds</code>
        </h3>
        <p>
          A different tone for each hit depending on how much damage you dealt.
          Set <strong>Setup → Game Options → Hit Sounds</strong> to Big Hits Low
          for lower pitch on bigger hits, or Big Hits High for higher pitch;
          Classic keeps the game's own sound.
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
          Trinity lets you force every enemy and every teammate to render as a
          model and skin you pick, with optional color overrides for the head,
          torso, legs, and rail.
        </p>
        <figure className="docs-hud-figure">
          <ZoomableImage
            src="/assets/play/cg_enemycolors.png"
            alt="A Trinity match with forced models active: two cyan Doom models in motion (forced enemy model + cg_enemyColors 333), a green Crash model in the background (forced team model + cg_teamColors 222), and two darkened bodies on the ground (cg_deadBodyDarken 1)."
          />
          <figcaption>
            The cyan Dooms are enemies, the green Crash is a teammate, and the
            darkened bodies on the ground are dead.
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
          position becomes the player&rsquo;s team color (red, blue, or white in
          FFA), so <code>cg_enemyColors "????"</code> tints every enemy and
          their rail in their team&rsquo;s color.
        </p>
        <p>
          The digits don't match the chat <code>^N</code> codes most players
          know from typing colored names, a long-standing Quake III quirk.{" "}
          <Link to="/docs/customize#color-codes">Color codes</Link> has the
          side-by-side translation.
        </p>
        <p>
          This system has no in-game menu;{" "}
          <Link to="/docs/customize#player-models">
            Forced enemy / team models
          </Link>{" "}
          in Customize has the cvars.
        </p>
      </div>
    </>
  );
}
