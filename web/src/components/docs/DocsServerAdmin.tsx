export function DocsServerAdmin() {
  return (
    <>
      <div className="about-section">
        <h2>Contributing stats to this hub</h2>
        <p>
          Server operators can run a Trinity collector alongside their q3
          server and publish match stats here. To get set up on{" "}
          <code>trinity.run</code>, ping <strong>NilClass</strong> in
          the{" "}
          <a href="https://discord.gg/tuDB2YNc7h">Team Beef Discord</a>{" "}
          with a proposed source name, then follow{" "}
          <a
            href="https://github.com/ernie/trinity-tracker/blob/main/docs/collector-setup.md"
            target="_blank"
            rel="noopener noreferrer"
          >
            docs/collector-setup.md
          </a>{" "}
          — the <code>scripts/install-collector.sh</code> bootstrap
          handles the engine + tracker install end-to-end.
        </p>
        <p>
          To participate, your q3 server must set:
        </p>
        <ul>
          <li>
            <code>g_logSync 1</code> — flushes log writes immediately so
            the collector tails events in real time.
          </li>
          <li>
            <code>g_trinityHandshake 1</code> — gates the server's data
            to verified Trinity clients. Without it the hub rejects
            match stats and hides the server from the dashboard cards
            and activity feed.
          </li>
          <li>
            <code>rconpassword "..."</code> — the collector uses RCON to
            deliver welcome messages and <code>!claim</code> /{" "}
            <code>!help</code> replies back to players. Without it, stats
            still flow but players see no in-game messages.
          </li>
        </ul>
        <p>Strongly recommended:</p>
        <ul>
          <li>
            <code>sv_tvAuto 1</code> — auto-record demos on every map
            load. Without it the hub has nothing to play back for matches
            recorded on your server.
          </li>
          <li>
            <code>sv_tvAutoMinPlayers 1</code> +{" "}
            <code>sv_tvAutoMinPlayersSecs 60</code> — discard recordings
            that never had a real human on the server for at least a
            minute, so you don't pile up bot-only matches.
          </li>
          <li>
            <code>g_overtimelimit 2</code> — cap sudden-death overtime
            at a couple of minutes. The default of <code>0</code> is
            unbounded sudden death, which can produce huge demo files
            (and matches that never end) if a tied match drags on.
          </li>
          <li>
            <code>sv_tvDownload 1</code> +{" "}
            <code>sv_dlURL "http://&lt;your-host&gt;:27970"</code> — let
            clients download recorded <code>.tvd</code> files and
            missing pk3s straight from your fast-dl vhost. The bundled{" "}
            <code>scripts/bootstrap-nginx.sh</code> stands up the
            matching <code>:27970</code> vhost; you still have to point
            your q3 server at it.
          </li>
        </ul>
      </div>

      <div className="about-section">
        <h2>Server CVars</h2>
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

      <div className="about-section">
        <h2>Trinity Handshake</h2>
        <p>
          Set <code>g_trinityHandshake 1</code> to require Trinity clients
          on your server. Non-Trinity clients will be disconnected after
          10 seconds.
        </p>
        <p>
          Players who have logged in to their Trinity account will
          automatically have their credentials verified during the
          handshake, linking their GUID to their account — no{" "}
          <code>!claim</code> or <code>!link</code> needed.
        </p>
      </div>
    </>
  );
}
