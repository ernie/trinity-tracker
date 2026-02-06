import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import App from './App'
import { PlayersPage, AccountPage, LeaderboardPage, MatchesPage, MatchDetailPage, AboutPage } from './components'
import { VerifiedPlayersProvider } from './hooks/useVerifiedPlayers'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <VerifiedPlayersProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<App />} />
          <Route path="/players" element={<PlayersPage />} />
          <Route path="/players/:id" element={<PlayersPage />} />
          <Route path="/matches" element={<MatchesPage />} />
          <Route path="/matches/:id" element={<MatchDetailPage />} />
          <Route path="/leaderboard" element={<LeaderboardPage />} />
          <Route path="/account" element={<AccountPage />} />
          <Route path="/about" element={<AboutPage />} />
        </Routes>
      </BrowserRouter>
    </VerifiedPlayersProvider>
  </StrictMode>,
)
