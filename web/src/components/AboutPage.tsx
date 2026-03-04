import { Link } from "react-router-dom";
import { Header } from "./Header";

export function AboutPage() {
  return (
    <div className="about-page">
      <Header title="About" className="about-header" />

      <div className="about-content">
        <div className="about-section">
          <h2>What is Trinity?</h2>
          <p>
            In short: a love letter to Quake 3 Arena, and the games of id
            Software, more generally. I'd argue that the rate at which PC gaming
            advanced during the 1990s has not really been matched since then. I
            don't think it would have happened nearly so quickly without id.
            Quake 3 Arena remains the best arena shooter of all time, in my
            opinion. Yes, I've played Unreal Tournament. I said what I said.
          </p>
          <Link to="/getting-started" className="getting-started-cta">
            Get started with Trinity
          </Link>
        </div>

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
            <a href="https://discord.gg/tuDB2YNc7h">Team Beef Discord</a>.
          </p>
        </div>

        <div className="about-section">
          <h2>Please don't sue me.</h2>
          <p>
            Trinity is not affiliated, associated, authorized, endorsed by, or
            in any way officially connected with Bethesda or id Software, or any
            of its subsidiaries or its affiliates. Quake 3, Quake 3 Arena, id,
            id Software, id Tech and related logos are registered trademarks or
            trademarks of id Software LLC in the U.S. and/or other countries.
            All Rights Reserved.
          </p>
        </div>
      </div>
    </div>
  );
}
