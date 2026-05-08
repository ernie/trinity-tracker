import { useCallback } from 'react'
import { useLocation } from 'react-router-dom'
import { StatusPill } from './StatusPill'
import { ActivityDrawer } from './ActivityDrawer'
import { PlayerStatsModal } from './PlayerStatsModal'
import { PasswordChangeModal } from './PasswordChangeModal'
import { CommandPalette } from './CommandPalette'
import { useLiveData } from '../contexts/LiveDataContext'
import { useAuth } from '../hooks/useAuth'
import { useHotkey } from '../hooks/useHotkey'

// Universal UI rendered above every route: live-state indicators, the
// activity drawer, the global modals, and the ⌘K command palette.
export function Chrome() {
  const live = useLiveData()
  const { auth, changePassword } = useAuth()
  const { pathname } = useLocation()

  // The landing page has its own bespoke hero header with an inline live-count
  // pill, so hide the floating chrome-top-right cluster there to avoid the
  // duplicate StatusPill colliding with the hero brand.
  const isLanding = pathname === '/'

  // ⌘K / Ctrl+K opens the palette from anywhere.
  useHotkey({ key: 'k', meta: true }, useCallback(() => {
    live.setCommandPaletteOpen(true)
  }, [live]))

  return (
    <>
      {!isLanding && (
        <div className="chrome-top-right" aria-hidden={false}>
          <StatusPill
            humansOnline={live.activeHumanPlayersCount}
            activeServers={live.activeServersCount}
            isConnected={live.isConnected}
            open={live.activityDrawerOpen}
            onToggle={live.toggleActivityDrawer}
          />
        </div>
      )}

      {live.activityDrawerOpen && (
        <ActivityDrawer
          activities={live.activities}
          servers={live.servers}
          onClose={() => live.setActivityDrawerOpen(false)}
          onPlayerClick={live.showPlayer}
        />
      )}

      {live.selectedPlayer && (
        <PlayerStatsModal
          playerName={live.selectedPlayer.name}
          playerId={live.selectedPlayer.playerId}
          onClose={live.closePlayer}
        />
      )}

      {live.showPasswordChange && (
        <PasswordChangeModal
          required={auth.passwordChangeRequired}
          onPasswordChange={changePassword}
          onClose={() => live.setShowPasswordChange(false)}
        />
      )}

      <CommandPalette
        open={live.commandPaletteOpen}
        onClose={() => live.setCommandPaletteOpen(false)}
      />
    </>
  )
}
