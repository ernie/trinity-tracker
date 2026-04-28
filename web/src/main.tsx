import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import App from './App'
import { PlayersPage, AccountPage, LeaderboardPage, MatchesPage, MatchDetailPage, DemoPlayerPage, PlayPage, DocsPage, ClaimPage } from './components'
import { DocsGettingStarted } from './components/docs/DocsGettingStarted'
import { DocsFeatures } from './components/docs/DocsFeatures'
import { DocsServerAdmin } from './components/docs/DocsServerAdmin'
import { DocsCredits } from './components/docs/DocsCredits'
import { AdminPage } from './components/admin/AdminPage'
import { AdminUsers } from './components/admin/AdminUsers'
import { AdminSessions } from './components/admin/AdminSessions'
import { AdminPlayers } from './components/admin/AdminPlayers'
import { AdminSources } from './components/admin/AdminSources'
import { AdminAudit } from './components/admin/AdminAudit'
import { AuthProvider } from './hooks/useAuth'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<App />} />
          <Route path="/players" element={<PlayersPage />} />
          <Route path="/players/:id" element={<PlayersPage />} />
          <Route path="/matches" element={<MatchesPage />} />
          <Route path="/matches/:id/demo" element={<DemoPlayerPage />} />
          <Route path="/play" element={<PlayPage />} />
          <Route path="/matches/:id" element={<MatchDetailPage />} />
          <Route path="/leaderboard" element={<LeaderboardPage />} />
          <Route path="/account" element={<AccountPage />} />
          <Route path="/docs" element={<DocsPage />}>
            <Route index element={<Navigate to="/docs/getting-started" replace />} />
            <Route path="getting-started" element={<DocsGettingStarted />} />
            <Route path="features" element={<DocsFeatures />} />
            <Route path="server-admin" element={<DocsServerAdmin />} />
            <Route path="credits" element={<DocsCredits />} />
          </Route>
          <Route path="/admin" element={<AdminPage />}>
            <Route index element={<Navigate to="/admin/users" replace />} />
            <Route path="users" element={<AdminUsers />} />
            <Route path="sessions" element={<AdminSessions />} />
            <Route path="players" element={<AdminPlayers />} />
            <Route path="sources" element={<AdminSources />} />
            <Route path="audit" element={<AdminAudit />} />
          </Route>
          <Route path="/about" element={<Navigate to="/docs" replace />} />
          <Route path="/getting-started" element={<Navigate to="/docs/getting-started" replace />} />
          <Route path="/claim" element={<ClaimPage />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  </StrictMode>,
)
