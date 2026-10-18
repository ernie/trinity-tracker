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
          Server operators who want to connect a Quake III server to the Trinity
          network for stats, demos, and accounts.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="set-up-a-collector">Set up a collector</DocsH2>
        <div className="docs-tip">
          <p>
            <strong>What&rsquo;s a source?</strong> One host machine. Submit one
            request per machine you&rsquo;ll run, since each gets its own
            credential and its own rotate and leave controls. A name plus a
            region code, such as <code>mygame-jfk</code> or{" "}
            <code>mygame-fra</code>, works well if you run hosts in several
            regions.
          </p>
        </div>
        <p>Joining the network is self-service through this site:</p>
        <ol>
          <li>
            <strong>Sign in.</strong>{" "}
            <Link to="/docs/account">Create an account</Link> first if you
            haven&rsquo;t already.
          </li>
          <li>
            <strong>Request a source.</strong> Click{" "}
            <strong>Add servers</strong> at the top right. If you already have a
            source, click <strong>My servers</strong>, then{" "}
            <strong>Add another source</strong>. Fill out the form:
            <ul>
              <li>
                <strong>Source name:</strong> 3–16 characters: letters, numbers,{" "}
                <code>_</code>, or <code>-</code>.
              </li>
              <li>
                <strong>What is this for?</strong> <em>(optional)</em>: anything
                the reviewer should know about you and your server.
              </li>
            </ul>
            New requests go through admin review; re-requesting a name you left
            before is approved automatically.
          </li>
          <li>
            <strong>Download the credential file.</strong> Once your request is
            approved, the source card in <strong>My servers</strong> shows a{" "}
            <strong>Download .creds</strong> button. The same drawer lets you
            rotate credentials or leave the network anytime.
          </li>
          <li>
            <strong>Run the installer.</strong> On a Linux host (Debian and
            Ubuntu are the main targets), put your downloaded{" "}
            <code>.creds</code> file in a working directory, with your retail
            Quake III Arena <code>pak0.pk3</code> renamed{" "}
            <code>q3-pak0.pk3</code> and, for Team Arena game types, the Team
            Arena <code>pak0.pk3</code> renamed <code>mp-pak0.pk3</code>. Then
            run this from inside it:
            <CopyableCommand>
              {
                "curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh | sudo bash"
              }
            </CopyableCommand>
            The wizard finds the files in your working directory and walks you
            through DNS, nginx and Let&rsquo;s Encrypt, the engine install, and
            the collector daemon.{" "}
            <a
              href="https://github.com/ernie/trinity-tracker/blob/main/docs/collector-setup.md"
              target="_blank"
              rel="noopener noreferrer"
            >
              Collector setup
            </a>{" "}
            is the full reference, including pak file names and what Team Arena
            game types need.
          </li>
          <li>
            <strong>Configure your Quake III server.</strong> A few cvars are
            required for your collector to take part, and a few more are
            recommended; see{" "}
            <a href="#required-server-cvars">Required server cvars</a>.
          </li>
        </ol>
        <p>
          Ask for help in <a href={DISCORD_INVITE_URL}>the Trinity Discord</a>.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="required-server-cvars">Required server cvars</DocsH2>
        <div className="docs-tip">
          <p>The install script sets all of these for you.</p>
        </div>
        <p>Four connect your collector to the hub:</p>
        <ul>
          <li>
            <code>g_logSync 1</code>: writes log lines immediately, so the
            collector sees events in real time.
          </li>
          <li>
            <code>sv_conTap 1</code>: gives you, and any admins you delegate, a
            remote console for your servers.{" "}
            <code>trinity console &lt;server&gt;</code> sends rcon commands or
            follows the live console from anywhere you can log in, without shell
            access to the game box. Stats flow without it.
          </li>
          <li>
            <code>g_trinityHandshake 1</code>: admits only verified Trinity
            clients. Without it, the hub rejects match stats and hides the
            server from dashboard cards and the activity feed. With it,
            non-Trinity clients are disconnected after 10 seconds, and logged-in
            Trinity players have their account credentials checked as they
            connect, which links their GUID to their account with no{" "}
            <code>!claim</code> or <code>!link</code>.
          </li>
          <li>
            <code>rconpassword "..."</code>: the collector uses RCON to send
            welcome messages and <code>!claim</code> and <code>!help</code>{" "}
            replies to players. Without it, stats still flow but players see no
            in-game messages.
          </li>
        </ul>
        <p>
          Three more make your matches watchable on this site, live and
          afterward, which the network expects of every server:
        </p>
        <ul>
          <li>
            <code>sv_tvAuto 1</code>: records a demo on every map load.
          </li>
          <li>
            <code>sv_tvLive 1</code>: broadcasts the match in progress so
            spectators can watch it live on this site.
          </li>
          <li>
            <code>sv_tvDownload 1</code> and{" "}
            <code>sv_dlURL "https://dl.&lt;your-host&gt;"</code>: let clients
            download recorded <code>.tvd</code> files, and any pk3s they lack,
            straight from your fast-download vhost.
          </li>
        </ul>
        <div className="docs-tip">
          <p>
            The bundled <code>scripts/bootstrap-nginx.sh</code> sets up the{" "}
            <code>dl.&lt;your-host&gt;</code> vhost with a Let&rsquo;s Encrypt
            certificate; you&rsquo;ll need a <code>dl.&lt;your-host&gt;</code> A
            or AAAA record pointing at the same box. The install wizard fills in{" "}
            <code>sv_dlURL</code> for you.
          </p>
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="recommended-server-cvars">Recommended server cvars</DocsH2>
        <ul>
          <li>
            <code>sv_tvAutoMinPlayers 1</code> and{" "}
            <code>sv_tvAutoMinPlayersSecs 60</code>: discard recordings that
            never had a human on the server for at least a minute, so bot-only
            matches don&rsquo;t pile up.
          </li>
          <li>
            <code>g_overtimelimit N</code>: caps sudden-death overtime at N
            minutes. The default, <code>0</code>, is unbounded, and a tied match
            that drags on can produce huge demo files. A couple of minutes works
            for Tournament; CTF and other team modes often want more.
          </li>
          <li>
            <code>sv_pure 1</code>: a pure server running a VR-aware mod such as
            Trinity accepts VR players. A pure server running a mod without VR
            support sends VR players back to the main menu.
          </li>
          <li>
            <code>g_clanRotation 1</code>: winner-stays clan rotation in Team
            Arena game types. When a match ends, the losing team&rsquo;s clan
            rotates out, its roster bots replaced by the incoming clan&rsquo;s,
            and the next clan in line challenges the winner. It works only when{" "}
            <code>g_redteam</code> and <code>g_blueteam</code> name real Team
            Arena clans (Crusaders, Intruders, Pagans, Stroggs, The Fallen);
            custom team names turn clan rosters and rotation off. The install
            templates ship it on, with valid clan names.
          </li>
          <li>
            <code>g_grapple 1</code>: hands every player the grapple, starting
            at each player&rsquo;s next spawn, with no restart. Off by default;
            the trinity.run servers run with it on.
          </li>
          <li>
            <code>g_damage_gh N</code>: the damage an attached tether deals.
            Empty uses the game mode&rsquo;s value; <code>0</code> turns it off.{" "}
            <code>g_knockback_gh</code> does the same for the pad&rsquo;s
            knockback.
          </li>
          <li>
            <code>bot_grapple 0</code>: bots use the grapple whenever the server
            hands it out, both to get around and to fight; <code>0</code> keeps
            them off it. Getting around needs Trinity&rsquo;s navigation data (
            <code>.aat</code> files, shipped for the stock baseq3 and Team Arena
            maps); on other maps bots still fight with the grapple but travel
            without it. <code>2</code> logs bot grapple activity to the console.
          </li>
          <li>
            <code>sv_fps 40</code>: the Trinity default, and what the install
            script writes. The mod&rsquo;s movement and the grapple&rsquo;s pull
            are tuned for it, so a server left at the stock <code>20</code>{" "}
            plays a noticeably different game.
          </li>
        </ul>
      </div>
    </>
  );
}
