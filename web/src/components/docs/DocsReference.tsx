import { useMemo, useState } from "react";
import { DocsH2 } from "./DocsH2";
import { ReferenceSearch } from "./ReferenceSearch";
import { CvarTable } from "./CvarTable";
import { GlossaryList } from "./GlossaryList";
import { usePlatform, PLATFORM_LABELS } from "./PlatformContext";
import {
  PLAYER_CVARS,
  VR_CVARS,
  SERVER_CVARS,
  type CvarEntry,
} from "../../data/cvars";
import { GLOSSARY, type GlossaryEntry } from "../../data/glossary";

// Comprehensive lookup page — Trinity-introduced cvars + glossary,
// kept separate from the task-shaped tabs so newcomers don't think
// they need to read it. The page hoists two filter knobs:
//   1. a search string (matches name + description + default)
//   2. a "show all platforms" toggle — defaults to false, so each
//      reader sees only cvars relevant to their current platform.
//      Clicking the toggle in the sticky bar widens the view.
//
// Per-section counts annotate the headings while the search is
// active. Sections that contain no entries due to the platform
// filter (no search active) are hidden entirely so a Flatscreen
// reader doesn't see an empty VR CVars heading.
//
// Cvar inventory drawn from web/src/data/cvars.ts (verified against
// trinity-engine + trinity, trinity-vr, and trinity-quest sources). Glossary
// drawn from web/src/data/glossary.ts.
export function DocsReference() {
  const { platform } = usePlatform();
  const [filter, setFilter] = useState("");
  const [showAllPlatforms, setShowAllPlatforms] = useState(false);

  const filterLower = filter.trim().toLowerCase();
  const searching = filterLower.length > 0;

  // Apply the platform filter first, then the search filter. Glossary
  // and Server CVars aren't platform-tagged in a way that matters
  // here, so they only see the search filter.
  const filteredPlayer = useMemo(
    () =>
      PLAYER_CVARS.filter(
        (c) => showAllPlatforms || c.platforms.includes(platform),
      ).filter((c) => matchCvar(c, filterLower)),
    [filterLower, showAllPlatforms, platform],
  );
  const filteredVr = useMemo(
    () =>
      VR_CVARS.filter(
        (c) => showAllPlatforms || c.platforms.includes(platform),
      ).filter((c) => matchCvar(c, filterLower)),
    [filterLower, showAllPlatforms, platform],
  );
  const filteredServer = useMemo(
    () => SERVER_CVARS.filter((c) => matchCvar(c, filterLower)),
    [filterLower],
  );
  const filteredGlossary = useMemo(
    () => GLOSSARY.filter((g) => matchGlossary(g, filterLower)),
    [filterLower],
  );

  // Hide a section that the platform filter has emptied (no search
  // active). When searching, keep the section visible so the reader
  // sees a "no matches" body rather than wondering where the cvars
  // they expected went.
  const showPlayerSection = searching || filteredPlayer.length > 0;
  const showVrSection = searching || filteredVr.length > 0;

  return (
    <>
      <ReferenceSearch
        value={filter}
        onChange={setFilter}
        showAllPlatforms={showAllPlatforms}
        onToggleShowAll={() => setShowAllPlatforms((v) => !v)}
        currentPlatform={platform}
      />

      {!showAllPlatforms && (
        <p className="docs-reference__platform-note">
          Showing cvars relevant to <strong>{PLATFORM_LABELS[platform]}</strong>
          . Tap <em>Show all platforms</em> in the search bar to widen the view.
        </p>
      )}

      <div className="about-section">
        <DocsH2 id="glossary">
          <SectionHeader
            label="Glossary"
            count={filteredGlossary.length}
            filtering={searching}
          />
        </DocsH2>
        <p>
          Definitions of Trinity- and Quake-specific terms that come up across
          the docs. If you're skimming and a word looks foreign, it's probably
          here.
        </p>
        <GlossaryList entries={filteredGlossary} />
      </div>

      <div className="about-section">
        <DocsH2 id="color-codes">Color codes</DocsH2>
        <p>
          Three things in Quake 3 use single-digit color codes, and they work
          independently — the color slider in the player-settings menu, the{" "}
          <code>color1</code> / <code>color2</code> cvars (and the force-model
          overrides built on them), and the <code>^N</code> chat / player-name
          escapes. The codes look the same on the surface but disagree about
          which digit means which color, so it's easy to write <code>"4"</code>{" "}
          in one place expecting red and get something else somewhere else. This
          section is the translation table.
        </p>

        <h3>The player-settings color slider</h3>
        <p>
          The in-game settings menu shows seven color swatches in rainbow
          spectrum order. Pick the color you want; the menu writes the
          corresponding digit to <code>color1</code> for you.
        </p>
        <div className="docs-table-scroll">
          <table className="docs-color-table">
            <thead>
              <tr>
                <th>Slot</th>
                <th>Visible swatch</th>
                <th>
                  Stored <code>color1</code>
                </th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>0</td>
                <td>red</td>
                <td>4</td>
              </tr>
              <tr>
                <td>1</td>
                <td>yellow</td>
                <td>6</td>
              </tr>
              <tr>
                <td>2</td>
                <td>green</td>
                <td>2</td>
              </tr>
              <tr>
                <td>3</td>
                <td>cyan</td>
                <td>3</td>
              </tr>
              <tr>
                <td>4</td>
                <td>blue</td>
                <td>1</td>
              </tr>
              <tr>
                <td>5</td>
                <td>magenta</td>
                <td>5</td>
              </tr>
              <tr>
                <td>6</td>
                <td>white</td>
                <td>7</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p>
          You don't have to memorize the digits — the slider is there precisely
          so you don't. The numbers matter when you're hand-editing a config or
          setting <code>cg_enemyColors</code>, both of which skip the slider.
        </p>

        <h3>
          <code>color1</code> / <code>color2</code> +{" "}
          <code>cg_enemyColors</code> / <code>cg_teamColors</code>
        </h3>
        <p>
          When you type these cvars by hand, the digit-to-color mapping is the
          table below. <code>color1</code> drives your rail core, weapon glow,
          and muzzle flash; <code>color2</code> drives the rail spiral.{" "}
          <code>cg_enemyColors</code> and <code>cg_teamColors</code> use the
          same digits in a five-character string (head, body, legs, color1,
          color2); omitted positions inherit the player's own settings.
        </p>
        <div className="docs-table-scroll">
          <table className="docs-color-table">
            <thead>
              <tr>
                <th>Digit</th>
                <th>Color</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <code>1</code>
                </td>
                <td>blue</td>
              </tr>
              <tr>
                <td>
                  <code>2</code>
                </td>
                <td>green</td>
              </tr>
              <tr>
                <td>
                  <code>3</code>
                </td>
                <td>cyan</td>
              </tr>
              <tr>
                <td>
                  <code>4</code>
                </td>
                <td>red</td>
              </tr>
              <tr>
                <td>
                  <code>5</code>
                </td>
                <td>magenta</td>
              </tr>
              <tr>
                <td>
                  <code>6</code>
                </td>
                <td>yellow</td>
              </tr>
              <tr>
                <td>
                  <code>7</code>
                </td>
                <td>white</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p>
          A <code>?</code> placeholder anywhere in the string becomes the digit
          for the player's team color at runtime — <code>'4'</code> on red,{" "}
          <code>'1'</code> on blue, <code>'7'</code> in FFA — so one config
          string stays team-aware across matches.
        </p>

        <h3>
          Chat / name <code>^N</code> color escapes
        </h3>
        <p>
          For text — chat, player names, frag feed, scoreboard, menus — escapes
          like <code>^1text</code> use a different mapping entirely. This is the
          one most players have memorized from years of typing colored names and
          chat:
        </p>
        <div className="docs-table-scroll">
          <table className="docs-color-table">
            <thead>
              <tr>
                <th>Escape</th>
                <th>Color</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <code>^0</code>
                </td>
                <td>black</td>
              </tr>
              <tr>
                <td>
                  <code>^1</code>
                </td>
                <td>red</td>
              </tr>
              <tr>
                <td>
                  <code>^2</code>
                </td>
                <td>green</td>
              </tr>
              <tr>
                <td>
                  <code>^3</code>
                </td>
                <td>yellow</td>
              </tr>
              <tr>
                <td>
                  <code>^4</code>
                </td>
                <td>blue</td>
              </tr>
              <tr>
                <td>
                  <code>^5</code>
                </td>
                <td>cyan</td>
              </tr>
              <tr>
                <td>
                  <code>^6</code>
                </td>
                <td>magenta</td>
              </tr>
              <tr>
                <td>
                  <code>^7</code>
                </td>
                <td>white</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p>
          Only <code>2</code> (green) and <code>7</code> (white) mean the same
          color in both this table and the cvar table above. Everything else
          differs — most painfully, <code>1</code> and <code>4</code> swap. Chat{" "}
          <code>^1</code> is red, but <code>color1 1</code> is blue. Chat{" "}
          <code>^4</code> is blue, but <code>color1 4</code> is red. If you put{" "}
          <code>^1MyName</code> in your player name expecting your rail to
          match, it won't.
        </p>
        <p>
          For names and chat that look the same to players on every engine, stay
          within <code>^0</code>–<code>^7</code>.
        </p>
      </div>

      {showPlayerSection && (
        <div className="about-section">
          <DocsH2 id="player-cvars">
            <SectionHeader
              label="Player CVars"
              count={filteredPlayer.length}
              filtering={searching}
            />
          </DocsH2>
          <p>
            Trinity-introduced client-side cvars — cgame, UI, and Trinity login.
            Place these in your <code>autoexec.cfg</code> if you want them to
            persist across launches.
          </p>
          <CvarTable
            entries={filteredPlayer}
            hidePlatforms={!showAllPlatforms}
          />
        </div>
      )}

      {showVrSection && (
        <div className="about-section">
          <DocsH2 id="vr-cvars">
            <SectionHeader
              label="VR CVars"
              count={filteredVr.length}
              filtering={searching}
            />
          </DocsH2>
          <p>
            VR comfort, control, and rendering cvars on PCVR (Trinity VR) and
            Quest (Trinity Quest). Per-controller button bindings (
            <code>vr_button_map_*</code>) and per-weapon offset strings (
            <code>vr_weapon_adjustment_N</code>) live in your starter autoexec
            config rather than here — they're typically tuned there once and
            forgotten.
          </p>
          <CvarTable entries={filteredVr} hidePlatforms={!showAllPlatforms} />
        </div>
      )}

      <div className="about-section">
        <DocsH2 id="server-cvars">
          <SectionHeader
            label="Server CVars"
            count={filteredServer.length}
            filtering={searching}
          />
        </DocsH2>
        <p>
          Server-side cvars: gameplay rules (<code>g_*</code>) plus
          Trinity-specific engine cvars (<code>sv_*</code>) for TV recording and
          HTTP downloads. The collector install script sets the required ones
          for you (see{" "}
          <a href="/docs/admin#required-server-cvars">
            Server Admin · Required server cvars
          </a>
          ).
        </p>
        <CvarTable entries={filteredServer} hidePlatforms />
      </div>
    </>
  );
}

interface SectionHeaderProps {
  label: string;
  count: number;
  filtering: boolean;
}

// Section heading content — the label on its own when no filter is
// active; "Label (N matching)" when the search filter is on.
function SectionHeader({ label, count, filtering }: SectionHeaderProps) {
  if (!filtering) return <>{label}</>;
  return (
    <>
      {label} <span className="docs-reference__count">({count} matching)</span>
    </>
  );
}

// Case-insensitive substring match on cvar name + description +
// default. Empty filter matches everything.
function matchCvar(entry: CvarEntry, filterLower: string): boolean {
  if (!filterLower) return true;
  return (
    entry.name.toLowerCase().includes(filterLower) ||
    entry.description.toLowerCase().includes(filterLower) ||
    entry.default.toLowerCase().includes(filterLower)
  );
}

function matchGlossary(entry: GlossaryEntry, filterLower: string): boolean {
  if (!filterLower) return true;
  return (
    entry.term.toLowerCase().includes(filterLower) ||
    entry.definition.toLowerCase().includes(filterLower)
  );
}
