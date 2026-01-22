import { useRef, useEffect, useState } from 'react'
import { useLogStream } from '../hooks/useLogStream'
import type { ServerStatus } from '../types'

interface LogViewerProps {
  server: ServerStatus | null
  token: string
}

export function LogViewer({ server, token }: LogViewerProps) {
  const { lines, isConnected, error } = useLogStream(server?.server_id ?? null, token)
  const [autoScroll, setAutoScroll] = useState(true)
  const outputRef = useRef<HTMLDivElement>(null)

  // Auto-scroll to bottom when new lines arrive
  useEffect(() => {
    if (autoScroll && outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight
    }
  }, [lines, autoScroll])

  // Detect if user scrolls up (disable auto-scroll)
  const handleScroll = () => {
    if (!outputRef.current) return
    const { scrollTop, scrollHeight, clientHeight } = outputRef.current
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50
    if (!isAtBottom && autoScroll) {
      setAutoScroll(false)
    }
  }

  if (!server) {
    return (
      <div className="log-viewer">
        <div className="log-placeholder">
          Select a server to view logs
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="log-viewer">
        <div className="log-error">
          {error}
        </div>
      </div>
    )
  }

  return (
    <div className="log-viewer">
      <div className="log-status-bar">
        <span className={`log-status ${isConnected ? 'connected' : 'disconnected'}`}>
          {isConnected ? 'Connected' : 'Connecting...'}
        </span>
        <label className="auto-scroll-toggle">
          <input
            type="checkbox"
            checked={autoScroll}
            onChange={(e) => setAutoScroll(e.target.checked)}
          />
          Auto-scroll
        </label>
      </div>
      <div className="log-content" ref={outputRef} onScroll={handleScroll}>
        {lines.length === 0 ? (
          <div className="log-empty">Waiting for log data...</div>
        ) : (
          lines.map((line, i) => (
            <div key={i} className="log-line">{line}</div>
          ))
        )}
      </div>
    </div>
  )
}
