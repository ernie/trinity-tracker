import { Link } from 'react-router-dom'
import { DocsH2 } from './DocsH2'

// New /docs/account page — extracted "Your Account" from
// DocsGettingStarted plus "Authentication" from DocsFeatures, both
// preserved as their own sub-sections. Tone polish + content merge
// land in a later phase.
export function DocsAccount() {
  return (
    <>
      <div className="about-section">
        <DocsH2 id="your-account">Your Account</DocsH2>
        <p>
          Type <code>!claim</code> in the in-game chat to create an account.
          You'll receive a code to set up your username and password. Once
          you have an account, log in via the main menu's Account option —
          your identity on that client will be linked automatically. Log in
          on each engine or device you play from to link them all.
        </p>
        <p>
          Without an account, each installation has its own identity based
          on a <code>qkey</code> file. To keep your stats connected across
          devices, you'll need to copy your <code>qkey</code> between
          installations, set <code>cl_guidServerUniq 0</code> so your
          identity stays the same across servers, and use{" "}
          <code>!link</code> to merge any separate identities.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="authentication">Authentication</DocsH2>
        <p>
          Logging in links your game identity with your Trinity account,
          so your stats are automatically associated no matter which device
          or engine you play from.
        </p>
        <p>
          The easiest way to log in is from the main menu's{" "}
          <strong>Account</strong> option. You can
          also set it up manually by adding both <code>cl_trinityUser</code>{" "}
          and <code>cl_trinityToken</code> to your autoexec. Your game token
          can be found on your <Link to="/account">Account page</Link>.
        </p>
        <p>
          Once logged in, your credentials are included in the Trinity
          handshake when you connect to a server. Your GUID is automatically
          linked to your account, so you don't need to use{" "}
          <code>!claim</code> or <code>!link</code> for identities you play
          on while authenticated.
        </p>
      </div>
    </>
  )
}
