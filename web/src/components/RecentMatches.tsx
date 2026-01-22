import { useEffect, useState } from 'react'
import type { MatchSummary } from '../types'
import { MatchCard } from './MatchCard'

interface RecentMatchesProps {
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

export function RecentMatches({ onPlayerClick }: RecentMatchesProps) {
  const [matches, setMatches] = useState<MatchSummary[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    async function fetchMatches() {
      try {
        const res = await fetch('/api/matches?limit=6')
        if (res.ok) {
          const data = await res.json()
          setMatches(data ?? [])
        }
      } catch (e) {
        console.error('Failed to fetch matches:', e)
      } finally {
        setLoading(false)
      }
    }

    fetchMatches()
    const interval = setInterval(fetchMatches, 30000)
    return () => clearInterval(interval)
  }, [])

  if (loading) {
    return (
      <div className="recent-matches">
        <h2>Recent Matches</h2>
        <div className="loading-small">Loading...</div>
      </div>
    )
  }

  if (matches.length === 0) {
    return (
      <div className="recent-matches">
        <h2>Recent Matches</h2>
        <div className="no-matches">No matches yet</div>
      </div>
    )
  }

  return (
    <div className="recent-matches">
      <h2>Recent Matches</h2>
      <div className="matches-list">
        {matches.map((match) => (
          <MatchCard
            key={match.id}
            match={match}
            onPlayerClick={onPlayerClick}
            showPermalink
          />
        ))}
      </div>
    </div>
  )
}
