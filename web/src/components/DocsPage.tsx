import { NavLink, Outlet } from "react-router-dom";
import { Header } from "./Header";

const DOCS_TABS = [
  { path: "getting-started", label: "Getting Started" },
  { path: "features", label: "Features" },
  { path: "server-admin", label: "Server Admin" },
  { path: "credits", label: "Credits" },
];

export function DocsPage() {
  return (
    <div className="about-page">
      <Header title="Docs" className="about-header" />

      <div className="docs-tabs">
        {DOCS_TABS.map((tab) => (
          <NavLink
            key={tab.path}
            to={`/docs/${tab.path}`}
            className={({ isActive }) =>
              `docs-tab ${isActive ? "active" : ""}`
            }
          >
            {tab.label}
          </NavLink>
        ))}
      </div>

      <div className="about-content">
        <Outlet />

        <div className="about-section docs-disclaimer">
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
