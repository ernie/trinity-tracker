import { DocsH2 } from './DocsH2'

// New /docs/reference page — comprehensive cvar + glossary lookup,
// kept separate so newcomers don't think they need to read it.
// Phase 2 lifts the existing Server CVars list verbatim from
// DocsServerAdmin. Player CVars, the full glossary, and the in-page
// search input land in a later phase that walks each engine's
// Cvar_Get call sites.
export function DocsReference() {
  return (
    <>
      <div className="about-section">
        <DocsH2 id="glossary">Glossary</DocsH2>
        <p>
          A glossary of Trinity- and Quake-specific terms is in
          progress. For now, the comprehensive cvar reference below
          covers the server-side configuration surface.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="server-cvars">Server CVars</DocsH2>
        <p>
          These cvars are available for server admins running Trinity.
        </p>
        <ul>
          <li>
            <code>g_overtimelimit</code> (default: <code>0</code>) — overtime
            duration in minutes for tied matches (0 = sudden death)
          </li>
          <li>
            <code>g_teamDMSpawnThreshold</code> (default: <code>8</code>) —
            use team/CTF spawn points on maps with fewer FFA spawns than this
            value (avoids telefrag-fests in Team DM)
          </li>
          <li>
            <code>g_gameplay</code> (default: <code>0</code>) — gameplay
            rules: 0 = Quake 3, 1 = CPMA, 2 = Quake Live
          </li>
          <li>
            <code>g_movement</code> (default: <code>0</code>) — movement
            physics: 0 = Quake 3, 1 = CPMA, 2 = Quake Live, 3 = Quake Live
            Turbo
          </li>
          <li>
            <code>g_rotation</code> (default: empty) — path to a map rotation
            file for automated map cycling
          </li>
          <li>
            <code>g_trinityHandshake</code> (default: <code>0</code>) —
            require Trinity clients; non-Trinity clients are disconnected
            after 10 seconds
          </li>
          <li>
            <code>sv_tvAuto</code> (default: <code>0</code>) — automatically
            start TV recording on map load
          </li>
          <li>
            <code>sv_tvAutoMinPlayers</code> (default: <code>0</code>) —
            minimum concurrent non-spectator human players to keep an
            auto-recording (0 = always keep)
          </li>
          <li>
            <code>sv_tvAutoMinPlayersSecs</code> (default: <code>0</code>) —
            seconds the player threshold must be continuously met (0 =
            instantaneous)
          </li>
          <li>
            <code>sv_tvpath</code> (default: <code>demos</code>) — directory
            for TV recordings
          </li>
          <li>
            <code>sv_tvDownload</code> (default: <code>0</code>) — notify
            clients to download TV recordings via HTTP at end of match
            (requires <code>sv_dlURL</code>)
          </li>
          <li>
            <code>sv_dlURL</code> (default: empty) — base URL for HTTP
            downloads (pk3 and tvd)
          </li>
        </ul>
      </div>
    </>
  )
}
