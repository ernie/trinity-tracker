import { Link } from "react-router-dom";
import { DISCORD_INVITE_URL } from "../../constants/discord";
import { DocsH2 } from "./DocsH2";
import { CopyableCommand } from "./CopyableCommand";

export function DocsServerAdmin() {
  return (
    <>
      <div className="about-section">
        <DocsH2 id="who-this-is-for">Who this is for</DocsH2>
        <p>
          Trinity is open to any Q3 server operator who wants to participate. If
          you run (or are thinking about running) a Quake 3 server and want
          match stats, demos, and player accounts to flow through the Trinity
          network, this page walks you through it. If you're just here to play,
          you can skip this section and head back to{" "}
          <Link to="/docs">Welcome</Link>.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="set-up-a-collector">Set up a collector</DocsH2>
        <div className="docs-tip">
          <p>
            <strong>What's a source?</strong> One host machine — submit one
            request per machine you'll run, since each gets its own credential
            and independent rotate / leave controls. A name + region code (e.g.{" "}
            <code>mygame-jfk</code>, <code>mygame-fra</code>) is a good
            convention if you'll run hosts in multiple regions.
          </p>
        </div>
        <p>Joining the network is self-service through the hub:</p>
        <ol>
          <li>
            <strong>Sign in.</strong>{" "}
            <Link to="/docs/account">Create an account</Link> first if you
            haven't already.
          </li>
          <li>
            <strong>Request a source.</strong> Click <strong>My Servers</strong>{" "}
            (top-right when logged in) and fill out the <em>Add Servers</em>{" "}
            form:
            <ul>
              <li>
                <strong>Name</strong> — 3–16 characters, letters, numbers,{" "}
                <code>_</code>, or <code>-</code>. Shorter is better.
              </li>
              <li>
                <strong>Purpose</strong> <em>(optional)</em> — if you're not
                already a familiar face on the Discord, this is a good place to
                introduce yourself: who you are, what your server's about,
                anything that helps the reviewing admin recognize you.
              </li>
            </ul>
            New requests go through admin review; re-requesting a name you
            previously left is auto-approved.
          </li>
          <li>
            <strong>Download the credential file.</strong> Once your request is
            approved, the source card in <strong>My Servers</strong> shows a{" "}
            <strong>Download .creds</strong> button. The same drawer also lets
            you rotate creds or leave the network anytime.
          </li>
          <li>
            <strong>Run the installer.</strong> On a Linux host (Debian and
            Ubuntu primary): drop your downloaded <code>.creds</code> file and
            your retail Quake 3 <code>pak0.pk3</code>(s) into a working
            directory, then run from inside it:
            <CopyableCommand>
              {
                "curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh | sudo bash"
              }
            </CopyableCommand>
            The wizard auto-discovers the files in your working directory and
            walks DNS, nginx + Let's Encrypt, the trinity-engine install, and
            the collector daemon. Full reference (including pak filename
            conventions and what's needed for Team Arena gametypes):{" "}
            <a
              href="https://github.com/ernie/trinity-tracker/blob/main/docs/collector-setup.md"
              target="_blank"
              rel="noopener noreferrer"
            >
              docs/collector-setup.md
            </a>
            .
          </li>
          <li>
            <strong>Configure your q3 server.</strong> A handful of cvars need
            to be set for your collector to participate, and a couple more are
            strongly recommended. See <a href="#required-server-cvars">below</a>
            .
          </li>
        </ol>
        <p>
          Questions, troubleshooting, or just saying hi:{" "}
          <a href={DISCORD_INVITE_URL}>the Trinity Discord</a> is where the
          community lives.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="required-server-cvars">Required server cvars</DocsH2>
        <div className="docs-tip">
          <p>
            The install script sets all of these for you. They're listed here so
            you know what they do and don't accidentally remove them from your
            config later.
          </p>
        </div>
        <p>Four for wiring your collector into the hub:</p>
        <ul>
          <li>
            <code>g_logSync 1</code> — flushes log writes immediately so the
            collector tails events in real time.
          </li>
          <li>
            <code>sv_conTap 1</code> — gives you (and any admins you delegate) a
            remote console for your servers:{" "}
            <code>trinity console &lt;server&gt;</code> sends rcon commands or
            follows the live console from anywhere you can log in, no shell
            access to the game box needed. Stats still flow without it, but you
            lose remote console access.
          </li>
          <li>
            <code>g_trinityHandshake 1</code> — gates the server to verified
            Trinity clients. Without it, the hub rejects match stats and hides
            the server from dashboard cards and the activity feed. With it,
            non-Trinity clients are disconnected after 10 seconds, and logged-in
            Trinity players have their account credentials verified
            automatically during the handshake — their GUID is linked to their
            account with no <code>!claim</code> or <code>!link</code> needed.
          </li>
          <li>
            <code>rconpassword "..."</code> — the collector uses RCON to deliver
            welcome messages and <code>!claim</code> / <code>!help</code>{" "}
            replies back to players. Without it, stats still flow but players
            see no in-game messages.
          </li>
        </ul>
        <p>
          Three more so matches played on your server are watchable through the
          hub — network participation comes with the expectation that matches
          can be watched, live and after the fact:
        </p>
        <ul>
          <li>
            <code>sv_tvAuto 1</code> — auto-record demos on every map load.
          </li>
          <li>
            <code>sv_tvLive 1</code> — broadcast the in-progress match so
            spectators can watch it live on the hub while it's being played.
          </li>
          <li>
            <code>sv_tvDownload 1</code> +{" "}
            <code>sv_dlURL "https://dl.&lt;your-host&gt;"</code> — let clients
            download recorded <code>.tvd</code> files (and missing pk3s)
            straight from your fast-dl vhost.
          </li>
        </ul>
        <div className="docs-tip">
          <p>
            The bundled <code>scripts/bootstrap-nginx.sh</code> stands up the{" "}
            <code>dl.&lt;your-host&gt;</code> vhost with a Let's Encrypt cert
            (you'll need a <code>dl.&lt;your-host&gt;</code> A/AAAA record on
            the same box). The install wizard fills in <code>sv_dlURL</code> for
            you.
          </p>
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="recommended-server-cvars">Recommended server cvars</DocsH2>
        <p>
          Knobs for protecting your disk and shaping how matches play. Tune to
          your setup:
        </p>
        <ul>
          <li>
            <code>sv_tvAutoMinPlayers 1</code> +{" "}
            <code>sv_tvAutoMinPlayersSecs 60</code> — discard recordings that
            never had a real human on the server for at least a minute, so you
            don't pile up bot-only matches.
          </li>
          <li>
            <code>g_overtimelimit N</code> — cap sudden-death overtime at N
            minutes. The default of <code>0</code> is unbounded, which can
            produce huge demo files (and matches that never end) if a tied match
            drags on. Pick a value that fits your mode — a couple of minutes
            works for duels; CTF and other team modes often want more.
          </li>
          <li>
            <code>sv_pure 1</code> — require clients to run the game code from
            your pk3s rather than loose files of their own. Trinity servers
            historically ran <code>sv_pure 0</code> because VR clients could
            only use the game code compiled into them; they load QVMs now, so
            pure mode no longer locks VR players out. It does mean every client
            needs your exact paks — leave it at <code>0</code> if you serve a
            mixed crowd you'd rather not have to keep in sync.
          </li>
          <li>
            <code>g_clanRotation 1</code> — winner-stays clan rotation on Team
            Arena gametypes. When a match ends, the losing team's clan rotates
            out — its roster bots are swapped for the incoming clan's — and the
            next clan in line challenges the winner. It only engages when{" "}
            <code>g_redteam</code> / <code>g_blueteam</code> name real Team
            Arena clans (Crusaders, Intruders, Pagans, Stroggs, The Fallen);
            custom team names quietly turn clan rosters and rotation off. The
            install templates ship it enabled with valid clan names.
          </li>
        </ul>
      </div>
    </>
  );
}
