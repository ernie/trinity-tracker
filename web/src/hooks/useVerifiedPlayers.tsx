import { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react'
import type { VerifiedPlayer } from '../types'

interface VerifiedPlayersContextType {
  isVerifiedById: (playerId: number) => boolean
  isAdminById: (playerId: number) => boolean
  refresh: () => Promise<void>
}

const VerifiedPlayersContext = createContext<VerifiedPlayersContextType | null>(null)

export function useVerifiedPlayers(): VerifiedPlayersContextType {
  const context = useContext(VerifiedPlayersContext)
  if (!context) {
    throw new Error('useVerifiedPlayers must be used within a VerifiedPlayersProvider')
  }
  return context
}

interface VerifiedPlayersProviderProps {
  children: ReactNode
}

export function VerifiedPlayersProvider({ children }: VerifiedPlayersProviderProps) {
  // Map of player_id to is_admin
  const [verifiedPlayers, setVerifiedPlayers] = useState<Map<number, boolean>>(new Map())

  const fetchVerifiedPlayers = useCallback(async () => {
    try {
      const res = await fetch('/api/players/verified')
      if (res.ok) {
        const data: VerifiedPlayer[] = await res.json()
        const map = new Map<number, boolean>()
        for (const p of data) {
          map.set(p.player_id, p.is_admin)
        }
        setVerifiedPlayers(map)
      }
    } catch {
      // Ignore errors, verified badges are non-critical
    }
  }, [])

  useEffect(() => {
    fetchVerifiedPlayers()
  }, [fetchVerifiedPlayers])

  const isVerifiedById = useCallback((playerId: number) => {
    return verifiedPlayers.has(playerId)
  }, [verifiedPlayers])

  const isAdminById = useCallback((playerId: number) => {
    return verifiedPlayers.get(playerId) === true
  }, [verifiedPlayers])

  const value: VerifiedPlayersContextType = {
    isVerifiedById,
    isAdminById,
    refresh: fetchVerifiedPlayers,
  }

  return (
    <VerifiedPlayersContext.Provider value={value}>
      {children}
    </VerifiedPlayersContext.Provider>
  )
}

export { VerifiedPlayersContext }
