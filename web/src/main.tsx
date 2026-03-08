import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import App from './App'
import { PlayersPage, AccountPage, LeaderboardPage, MatchesPage, MatchDetailPage, DemoPlayerPage, PlayPage, AboutPage, GettingStartedPage, ClaimPage } from './components'
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
          <Route path="/about" element={<AboutPage />} />
          <Route path="/getting-started" element={<GettingStartedPage />} />
          <Route path="/claim" element={<ClaimPage />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  </StrictMode>,
)
