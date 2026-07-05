import { DISCORD_INVITE_URL } from "../constants/discord";

// Top-level /credits page. Lifted content from the old
// /docs/credits — now lives outside the docs chrome since it's not
// documentation. Visual treatment is intentionally simple — credits
// are a thank-you, not a showcase.
export function CreditsPage() {
  return (
    <div className="credits-page">
      <h1 className="credits-page__title">Who made this?</h1>
      <p>
        I'm NilClass. Or, occasionally, I go by{" "}
        <a href="https://ernie.io">Ernie Miller</a>. But really, the folks who
        made this are the people who built the projects my work draws from:
      </p>
      <ul>
        <li>
          <a href="https://www.idsoftware.com/">id Software</a>:{" "}
          <a href="https://github.com/id-Software/Quake-III-Arena">
            Quake III Arena
          </a>
        </li>
        <li>
          Team Beef:{" "}
          <a href="https://github.com/Team-Beef-Studios/ioq3quest">
            Quake3Quest
          </a>
        </li>
        <li>
          RippeR37: <a href="https://github.com/rippeR37/q3vr/">Quake 3 VR</a>
        </li>
        <li>
          ec-: <a href="https://github.com/ec-/quake3e">Quake3e</a> and{" "}
          <a href="https://github.com/ec-/baseq3a">baseq3a</a>
        </li>
        <li>
          Kr3m:{" "}
          <a href="https://github.com/Kr3m/missionpackplus">missionpackplus</a>
        </li>
        <li>
          <a href="https://www.moddb.com/members/zertero">ZerTerO</a>:{" "}
          <a href="https://www.moddb.com/mods/high-quality-quake">
            High Quality Quake
          </a>
        </li>
        <li>
          Jay Dolan: <a href="https://github.com/jdolan/quetoo">Quetoo</a>
        </li>
        <li>
          Everyone involved in the{" "}
          <a href="https://github.com/ioquake/ioq3">ioquake3</a> project over
          the years
        </li>
      </ul>
      <p>
        If you'd like to connect, stop by the{" "}
        <a href={DISCORD_INVITE_URL}>Trinity Discord</a>.
      </p>

      <section className="credits-page__legal">
        <h2 className="credits-page__legal-title">Please don't sue me.</h2>
        <p>
          Trinity is not affiliated, associated, authorized, endorsed by, or in
          any way officially connected with Bethesda or id Software, or any of
          its subsidiaries or its affiliates. Quake 3, Quake 3 Arena, id, id
          Software, id Tech and related logos are registered trademarks or
          trademarks of id Software LLC in the U.S. and/or other countries. All
          Rights Reserved.
        </p>
      </section>
    </div>
  );
}
