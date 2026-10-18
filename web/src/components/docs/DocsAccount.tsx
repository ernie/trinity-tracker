import { Link } from "react-router-dom";
import { ColoredText } from "../ColoredText";
import { DocsH2 } from "./DocsH2";

export function DocsAccount() {
  return (
    <>
      <div className="about-section">
        <DocsH2 id="why-account">Why a Trinity account?</DocsH2>
        <p>
          Trinity tracks every player it sees, with or without an account. An
          account ties everything you do across machines to one identity:
        </p>
        <ul>
          <li>
            Sign in on every machine you play on, flatscreen or VR, and your
            stats count toward one profile.
          </li>
          <li>
            A green checkmark shows next to your name on the in-game scoreboard
            and on this site&rsquo;s match cards and leaderboards, so other
            players can tell which &ldquo;
            <ColoredText text="^1PhR4g^7M4st3r" />
            &rdquo; is which. Admins get a gold star instead.
          </li>
          <li>
            On Trinity servers, your in-game name is locked to your
            account&rsquo;s display name, and anyone else who joins with it is
            renamed automatically. The bots get the same protection: every stock
            Quake III and Team Arena character, clan rosters included, owns its
            name the way you own yours, so nobody can masquerade as Xaero
            either.
          </li>
          <li>
            Eligible players get custom announcer clips: your name when you join
            a server, and a win call when you take a match in FFA or 1v1. Clips
            are added by request once a player has at least 25 match victories.
          </li>
          <li>
            A new install or device links to your identity as soon as you sign
            in on it.
          </li>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="claim">Create your account</DocsH2>
        <p>
          The first step happens in-game on a Trinity server; the{" "}
          <Link to="/servers">Servers</Link> page labels them. Standard Quake
          III servers don&rsquo;t run Trinity&rsquo;s chat commands.
        </p>
        <ol>
          <li>
            Connect to a Trinity server. The first time you join, the server
            greets you with a hint:{" "}
            <em>
              &ldquo;Welcome, <ColoredText text="^1PhR4g^7M4st3r" />! Type
              !claim to link this identity to an account.&rdquo;
            </em>
          </li>
          <li>
            Type <code>!claim</code> in chat. The server replies with a
            six-digit code, for example:{" "}
            <em>
              &ldquo;Your claim code is: 482917. Visit trinity.run to claim this
              identity. Expires in 30 minutes.&rdquo;
            </em>
          </li>
          <li>
            Click <strong>Claim</strong> at the top of the page, enter your
            code, and confirm the player record is yours.
          </li>
          <li>
            Pick a display name, username, and password for your account. The
            display name supports Quake III color codes, such as{" "}
            <ColoredText text="^1Ph^7R4g" />, and you can restyle it anytime on
            your <Link to="/account">Account</Link> page. That links your
            in-game identity on that server to the account.
          </li>
        </ol>
        <div className="docs-tip">
          <p>
            <strong>Tip:</strong> if you play in VR, pick a shorter password
            when you first claim. You&rsquo;ll type it on the VR keyboard to log
            in on each VR install at least once, and copying and pasting is
            awkward in VR. You can change it later on your Account page in any
            browser, and your in-game login keeps working unless you rotate your
            game token.
          </p>
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="authentication">Log in</DocsH2>
        <p>
          Log in from <strong>Account</strong> on the main menu; Trinity saves
          the credentials to your config.
        </p>
        <p>
          Or add <code>cl_trinityUser</code> and <code>cl_trinityToken</code> to
          your <code>autoexec.cfg</code>. Your game token, a long generated
          string separate from your password, is on your{" "}
          <Link to="/account">Account</Link> page. Copy it once and paste it
          into the autoexec on every install you want logged in.
        </p>
        <p>
          Either way, once you&rsquo;re logged in, joining a Trinity server
          links that install to your account.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="link-devices">Link another device or install</DocsH2>
        <p>
          Log in on the new install, from the Account menu or with the autoexec
          cvars, and join a Trinity server. The install is linked to your
          account the first time you do.
        </p>
        <p>
          If an install doesn&rsquo;t link, generate a link code on your{" "}
          <Link to="/account">Account</Link> page, then type{" "}
          <code>!link &lt;code&gt;</code> in chat on that install, using the
          six-digit code the page shows.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="sessions-security">Sessions and security</DocsH2>
        <p>
          To end every session on this site at once, say on a shared computer
          you forgot to log out of, use <strong>Log out everywhere</strong> in
          the <strong>Security</strong> section of your{" "}
          <Link to="/account">Account</Link> page. Changing your password does
          the same.
        </p>
        <p>
          Neither affects your game token, so your installs stay logged in. If
          the game token itself leaked, rotate it on the same page, then update
          any autoexec you pasted it into.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="without-account">Playing without an account</DocsH2>
        <p>
          Without an account you still appear on this site. Trinity tracks
          players by <code>qkey</code>, a file each install generates, and your
          stats build up on whichever <code>qkey</code> you&rsquo;re using.
        </p>
        <p>
          Every install starts with its own <code>qkey</code>, so each new
          machine counts as a new player. To keep your stats together without an
          account, copy your <code>qkey</code> file between installs, keep{" "}
          <code>cl_guidServerUniq</code> at <code>0</code> (the default) so it
          stays the same on every server, and use <code>!link</code> to merge
          any identities that drift apart.
        </p>
      </div>
    </>
  );
}
