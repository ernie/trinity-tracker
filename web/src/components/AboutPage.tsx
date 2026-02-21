import { useState } from "react";
import { Link } from "react-router-dom";
import { AppLogo } from "./AppLogo";
import { PageNav } from "./PageNav";
import { LoginForm } from "./LoginForm";
import { UserManagement } from "./UserManagement";
import { useAuth } from "../hooks/useAuth";

export function AboutPage() {
  const { auth, login, logout } = useAuth();
  const [showUserManagement, setShowUserManagement] = useState(false);

  return (
    <div className="about-page">
      <header className="about-header">
        <h1>
          <AppLogo />
          About
        </h1>
        <PageNav />
        <div className="auth-section">
          {auth.isAuthenticated ? (
            <div className="user-info">
              {auth.isAdmin && (
                <button
                  onClick={() => setShowUserManagement(true)}
                  className="admin-btn"
                >
                  Users
                </button>
              )}
              <Link to="/account" className="username-link">
                {auth.username}
              </Link>
              <button onClick={logout} className="logout-btn">
                Logout
              </button>
            </div>
          ) : (
            <LoginForm
              onLogin={(username, password) => login({ username, password })}
            />
          )}
        </div>
      </header>

      <div className="about-content">
        <div className="about-section">
          <h2>What is Trinity?</h2>
          <p>
            In short: a love letter to Quake 3 Arena, and the games of id
            Software, more generally. I'd argue that the rate at which PC gaming
            advanced during the 1990s has not really been matched, since. I
            don't think it would have happened without id. Certainly, not as
            quickly.
          </p>
          <p>
            This whole journey started after I rediscovered one of the greats in
            VR, thanks to <a href="https://quake3.quakevr.com">Quake3Quest</a>,
            and then <a href="https://ripper37.github.io/q3vr/">Quake 3 VR</a>.
            I wanted to port some{" "}
            <a href="https://github.com/ec-/baseq3a">baseq3a</a> features over
            to it. That led to another idea, and another. And, well, here we
            are. I hope a new generation of players get to experience a Quake 3
            Arena even better than it was, originally, as a result. I regularly
            release updated builds of both for people to try over on the{" "}
            <a href="https://discord.gg/tuDB2YNc7h">Team Beef Discord</a>.
          </p>
          <p>What you see on this site is the combination of:</p>
          <ul>
            <li>
              <a href="https://github.com/ernie/trinity-tools">
                trinity-tracker
              </a>{" "}
              &mdash; this stats tracker
            </li>
            <li>
              <a href="https://github.com/ernie/trinity">trinity</a> &mdash;
              custom Quake 3 mod
            </li>
            <li>
              <a href="https://github.com/ernie/trinity-engine">
                trinity-engine
              </a>{" "}
              &mdash; custom build of Quake3e used on the server (and a custom
              flatscreen engine)
            </li>
          </ul>
        </div>

        <div className="about-section">
          <h2>Who made this?</h2>
          <p>
            I'm NilClass. Or, occasionally, I go by{" "}
            <a href="https://ernie.io">Ernie Miller</a>. But really, the folks
            who made this are the people who built the projects my work is based
            on:
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

      {showUserManagement && auth.isAdmin && auth.token && (
        <UserManagement
          token={auth.token}
          currentUsername={auth.username!}
          onClose={() => setShowUserManagement(false)}
        />
      )}
    </div>
  );
}
