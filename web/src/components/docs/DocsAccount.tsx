import { Link } from 'react-router-dom'
import { ColoredText } from '../ColoredText'
import { DocsH2 } from './DocsH2'

// /docs/account — Phase 3b rewrite. Replaces the Phase 2 verbatim
// lift from DocsGettingStarted + DocsFeatures with a focused
// "create → log in → link more devices" flow that matches the
// collector's actual !claim / !link UX (verified in
// internal/collector/manager.go and internal/storage/sqlite.go).
// The qkey-only path is mentioned briefly but framed as the harder
// option, so this page doesn't contradict Install's troubleshooting
// which steers stats-fragmentation users toward accounts.
export function DocsAccount() {
  return (
    <>
      <div className="about-section">
        <DocsH2 id="why-account">Why a Trinity account?</DocsH2>
        <p>
          Trinity tracks every player it sees, account or not — but
          a Trinity account ties everything you do across machines
          to one identity:
        </p>
        <ul>
          <li>
            <strong>Stats follow you.</strong> Flatscreen at the
            office (no, you'd never do that), PCVR at home, Quest
            on the road — sign in on each one and they all count
            toward the same profile.
          </li>
          <li>
            <strong>Verified player badge.</strong> A green
            checkmark shows up next to your name on the in-game
            scoreboard (and on match cards and leaderboards here
            on the site) so other players can tell at a glance
            which "<ColoredText text="^1PhR4g^7M4st3r" />" is
            which. Admins get a gold star instead.
          </li>
          <li>
            <strong>Name lock.</strong> On Trinity servers, your
            in-game name is locked to your account's display
            name — you always show up with a consistent identity
            across sessions and servers. No other registered
            player can claim the same name.
          </li>
          <li>
            <strong>Announcer clips.</strong> Eligible players get
            custom audio announcements — your name when you join a
            server, and a win call when you take a match in FFA or
            1v1. Clips are currently added by request once a player
            has at least 25 match victories.
          </li>
          <li>
            <strong>No <code>!link</code> commands to manage.</strong>{' '}
            New install, new device, fresh reinstall — sign in
            once and Trinity links your identity automatically.
          </li>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="claim">Create your account</DocsH2>
        <p>
          The first step happens in-game on a Trinity
          server (the ones labeled on the{' '}
          <Link to="/servers">Servers page</Link> — vanilla servers
          don't process Trinity's chat commands).
        </p>
        <ol>
          <li>
            Connect to a Trinity server. The first time
            you join, the server greets you with a hint:{' '}
            <em>
              "Welcome,{' '}
              <ColoredText text="^1PhR4g^7M4st3r" />! !claim to
              link your identity!"
            </em>
          </li>
          <li>
            Type <code>!claim</code> in chat. The server replies
            with a six-digit code, e.g.{' '}
            <em>"Your claim code is: 482917 — Visit trinity.run to
            claim this identity. Expires in 30 minutes."</em>
          </li>
          <li>
            Click <strong>Claim</strong> at the top of the page,
            enter your code, and confirm the player record is
            yours.
          </li>
          <li>
            Pick a username and password for your account. You're
            done — your in-game identity on that server is now
            linked.
          </li>
        </ol>
        <div className="docs-tip">
          <p>
            <strong>Tip:</strong> if you play in VR, pick a
            shorter password when you first claim. You'll need
            to type it on the VR virtual keyboard to log in
            on each VR install at least once, and copy/paste
            isn't easy in VR. You can change it later from your
            Account page in any browser — your in-game session
            stays valid unless you explicitly rotate your game
            token.
          </p>
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="authentication">Log in</DocsH2>
        <p>
          Once you have an account, log in from the in-game{' '}
          <strong>Account</strong> menu — the option lives on the
          main menu, same as any other Quake 3 menu item. Enter
          your username and password and Trinity stores them in
          your config.
        </p>
        <p>
          Power-user alternative: add{' '}
          <code>cl_trinityUser</code> and{' '}
          <code>cl_trinityToken</code> to your{' '}
          <code>autoexec.cfg</code> directly. Your game token (a
          long generated string, not your password) lives on your{' '}
          <Link to="/account">Account page</Link> — copy it once
          and paste it into the autoexec on every install you want
          pre-authenticated.
        </p>
        <p>
          Either way, once your client has valid credentials,
          Trinity bundles them into the handshake when you connect
          to a server, and the server's collector links your
          current identity to your account on the spot.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="link-devices">Link another device or install</DocsH2>
        <p>
          You don't need to do anything special — just log in on
          the new install (in-game Account menu, or autoexec
          cvars) and play. The first time you join a Trinity
          server with valid credentials, that install's identity
          gets bound to your account.
        </p>
        <p>
          If automatic linking misses for some reason, there's a
          manual fallback. From your{' '}
          <Link to="/account">Account page</Link>, generate a
          six-digit link code, then in-game on the install you
          want to link, type <code>!link 482917</code>{' '}
          (substituting your actual code). Same end result.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="without-account">Playing without an account</DocsH2>
        <p>
          If you'd rather not make an account, you'll still
          appear on the hub — Trinity tracks identities by{' '}
          <code>qkey</code> (a per-install file the engine
          generates) and your stats accumulate on whatever{' '}
          <code>qkey</code> you're currently using.
        </p>
        <p>
          The catch: every install starts with its own{' '}
          <code>qkey</code>, so each new machine is a fresh
          identity from the hub's perspective. To keep stats
          merged without an account, you'd need to copy your{' '}
          <code>qkey</code> file between installs, set{' '}
          <code>cl_guidServerUniq 0</code> to keep it stable
          across servers, or use <code>!link</code> to merge any
          identities that drifted apart. It works, but it's the
          kind of thing accounts exist to avoid — most players
          claim sooner or later.
        </p>
      </div>
    </>
  )
}
