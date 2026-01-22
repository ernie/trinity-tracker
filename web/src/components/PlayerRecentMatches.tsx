import { useEffect, useState } from 'react'
import type { MatchSummary } from '../types'
import { MatchCard } from './MatchCard'

const PAGE_SIZE = 6

interface PlayerRecentMatchesProps {
  playerId: number
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

export function PlayerRecentMatches({ playerId, onPlayerClick }: PlayerRecentMatchesProps) {
  const [matches, setMatches] = useState<MatchSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)

  useEffect(() => {
    async function fetchMatches() {
      try {
        setLoading(true)
        setMatches([])
        setHasMore(true)
        const res = await fetch(`/api/players/${playerId}/matches?limit=${PAGE_SIZE}`)
        if (res.ok) {
          const data = await res.json()
          const fetchedMatches = data ?? []
          setMatches(fetchedMatches)
          setHasMore(fetchedMatches.length === PAGE_SIZE)
        }
      } catch (e) {
        console.error('Failed to fetch player matches:', e)
      } finally {
        setLoading(false)
      }
    }

    if (playerId) {
      fetchMatches()
    }
  }, [playerId])

  const loadMore = async () => {
    if (loadingMore || !hasMore || matches.length === 0) return

    const lastMatchId = matches[matches.length - 1].id
    try {
      setLoadingMore(true)
      const res = await fetch(`/api/players/${playerId}/matches?limit=${PAGE_SIZE}&before=${lastMatchId}`)
      if (res.ok) {
        const data = await res.json()
        const newMatches = data ?? []
        setMatches(prev => [...prev, ...newMatches])
        setHasMore(newMatches.length === PAGE_SIZE)
      }
    } catch (e) {
      console.error('Failed to fetch more matches:', e)
    } finally {
      setLoadingMore(false)
    }
  }

  if (loading) {
    return (
      <div className="recent-matches player-recent-matches">
        <h2>Recent Matches</h2>
        <div className="loading-small">Loading...</div>
      </div>
    )
  }

  if (matches.length === 0) {
    return (
      <div className="recent-matches player-recent-matches">
        <h2>Recent Matches</h2>
        <div className="no-matches">No matches yet</div>
      </div>
    )
  }

  return (
    <div className="recent-matches player-recent-matches">
      <h2>Recent Matches</h2>
      <div className="matches-list">
        {matches.map((match) => (
          <MatchCard
            key={match.id}
            match={match}
            onPlayerClick={onPlayerClick}
            highlightPlayerId={playerId}
            showPermalink
          />
        ))}
      </div>
      {hasMore && (
        <div className="load-more-container">
          <button
            className="load-more-btn"
            onClick={loadMore}
            disabled={loadingMore}
          >
            {loadingMore ? 'Loading...' : 'Load More'}
          </button>
        </div>
      )}
    </div>
  )
}
