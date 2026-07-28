import { useMemo, useState } from "react";
import { Navigate, useLocation } from "react-router-dom";
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
  const location = useLocation();
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

  // Color codes lives on Customize now, next to the cvars that use it.
  if (location.hash === "#color-codes") {
    return <Navigate to="/docs/customize#color-codes" replace />;
  }

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
