import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import App from "./App";
import {
  LandingPage,
  PlayersPage,
  AccountPage,
  LeaderboardPage,
  MatchesPage,
  MatchDetailPage,
  DemoPlayerPage,
  PlayPage,
  DocsPage,
  ClaimPage,
} from "./components";
import { Quake3EulaPage } from "./components/Quake3EulaPage";
import { DocsServerAdmin } from "./components/docs/DocsServerAdmin";
import { DocsWelcome } from "./components/docs/DocsWelcome";
import { DocsInstall } from "./components/docs/DocsInstall";
import { DocsPlay } from "./components/docs/DocsPlay";
import { DocsAccount } from "./components/docs/DocsAccount";
import { DocsCustomize } from "./components/docs/DocsCustomize";
import { DocsReference } from "./components/docs/DocsReference";
import { LivePlayerPage } from "./components/LivePlayerPage";
import { CreditsPage } from "./components/CreditsPage";
import { AdminPage } from "./components/admin/AdminPage";
import { AdminUsers } from "./components/admin/AdminUsers";
import { AdminSessions } from "./components/admin/AdminSessions";
import { AdminPlayers } from "./components/admin/AdminPlayers";
import { AdminSources } from "./components/admin/AdminSources";
import { AdminAudit } from "./components/admin/AdminAudit";
import { AuthProvider } from "./hooks/useAuth";
import { LiveDataProvider } from "./contexts/LiveDataContext";
import { Chrome } from "./components/Chrome";
import { Footer } from "./components/Footer";
import { ScrollToTop } from "./components/ScrollToTop";
import { RouteHeader } from "./components/RouteHeader";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AuthProvider>
      <BrowserRouter>
        <LiveDataProvider>
          <ScrollToTop />
          <Chrome />
          <div className="app-shell">
            <RouteHeader />
            <main className="app-main">
              <Routes>
                <Route path="/" element={<LandingPage />} />
                <Route path="/servers" element={<App />} />
                <Route path="/players" element={<PlayersPage />} />
                <Route path="/players/:id" element={<PlayersPage />} />
                <Route path="/matches" element={<MatchesPage />} />
                <Route path="/matches/:id/demo" element={<DemoPlayerPage />} />
                <Route path="/tv/:source/:key" element={<LivePlayerPage />} />
                <Route path="/play" element={<PlayPage />} />
                <Route path="/matches/:id" element={<MatchDetailPage />} />
                <Route path="/leaderboard" element={<LeaderboardPage />} />
                <Route path="/account" element={<AccountPage />} />
                <Route path="/docs" element={<DocsPage />}>
                  <Route index element={<DocsWelcome />} />
                  <Route path="install" element={<DocsInstall />} />
                  <Route path="play" element={<DocsPlay />} />
                  <Route path="account" element={<DocsAccount />} />
                  <Route path="customize" element={<DocsCustomize />} />
                  <Route path="admin" element={<DocsServerAdmin />} />
                  <Route path="reference" element={<DocsReference />} />
                </Route>
                <Route path="/credits" element={<CreditsPage />} />
                <Route path="/admin" element={<AdminPage />}>
                  <Route
                    index
                    element={<Navigate to="/admin/users" replace />}
                  />
                  <Route path="users" element={<AdminUsers />} />
                  <Route path="sessions" element={<AdminSessions />} />
                  <Route path="players" element={<AdminPlayers />} />
                  <Route path="sources" element={<AdminSources />} />
                  <Route path="audit" element={<AdminAudit />} />
                </Route>
                <Route path="/claim" element={<ClaimPage />} />
                <Route path="/quake3-eula" element={<Quake3EulaPage />} />
              </Routes>
            </main>
            <Footer />
          </div>
        </LiveDataProvider>
      </BrowserRouter>
    </AuthProvider>
  </StrictMode>,
);
