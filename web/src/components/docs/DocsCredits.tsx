import { DISCORD_INVITE_URL } from "../../constants/discord";

export function DocsCredits() {
  return (
    <div className="about-section">
      <h2>Who made this?</h2>
      <p>
        I'm NilClass. Or, occasionally, I go by{" "}
        <a href="https://ernie.io">Ernie Miller</a>. But really, the folks
        who made this are the people who built the projects my work is based
        on:
      </p>
      <ul>
        <li>
          Team Beef:{" "}
          <a href="https://github.com/Team-Beef-Studios/ioq3quest">
            Quake3Quest
          </a>
        </li>
        <li>
          RippeR37:{" "}
          <a href="https://github.com/rippeR37/q3vr/">Quake 3 VR</a>
        </li>
        <li>
          ec-: <a href="https://github.com/ec-/quake3e">Quake3e</a> and{" "}
          <a href="https://github.com/ec-/baseq3a">baseq3a</a>
        </li>
        <li>
          Kr3m:{" "}
          <a href="https://github.com/Kr3m/missionpackplus">
            missionpackplus
          </a>
        </li>
        <li>
          Everyone involved in the{" "}
          <a href="https://github.com/ioquake/ioq3">ioquake3</a> project
          over the years
        </li>
      </ul>
      <p>
        If you'd like to connect, stop by the{" "}
        <a href={DISCORD_INVITE_URL}>Trinity Discord</a>.
      </p>
    </div>
  );
}
