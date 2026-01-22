import { useEffect, useRef, useState } from 'react'

interface LogMessage {
  type: 'initial' | 'lines' | 'error'
  lines?: string[]
  message?: string
}

interface UseLogStreamResult {
  lines: string[]
  isConnected: boolean
  error: string | null
}

const MAX_LINES = 1000

export function useLogStream(serverId: number | null, token: string | null): UseLogStreamResult {
  const [lines, setLines] = useState<string[]>([])
  const [isConnected, setIsConnected] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimeoutRef = useRef<number | null>(null)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true

    const connect = () => {
      if (!serverId || !token || !mountedRef.current) return
      if (wsRef.current?.readyState === WebSocket.OPEN) return

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${protocol}//${window.location.host}/ws/logs?token=${encodeURIComponent(token)}&server_id=${serverId}`

      const ws = new WebSocket(url)
      wsRef.current = ws

      ws.onopen = () => {
        if (!mountedRef.current) return
        setIsConnected(true)
        setError(null)
        console.log('Log WebSocket connected')
      }

      ws.onclose = () => {
        if (!mountedRef.current) return
        setIsConnected(false)
        console.log('Log WebSocket disconnected, reconnecting in 3s...')
        reconnectTimeoutRef.current = window.setTimeout(connect, 3000)
      }

      ws.onerror = () => {
        if (!mountedRef.current) return
        setError('Connection error')
      }

      ws.onmessage = (event) => {
        if (!mountedRef.current) return
        try {
          const msg = JSON.parse(event.data) as LogMessage

          if (msg.type === 'error') {
            setError(msg.message || 'Unknown error')
            return
          }

          if (msg.type === 'initial' && msg.lines) {
            setLines(msg.lines.slice(-MAX_LINES))
          } else if (msg.type === 'lines' && msg.lines) {
            setLines(prev => {
              const newLines = [...prev, ...msg.lines!]
              return newLines.slice(-MAX_LINES)
            })
          }
        } catch (e) {
          console.error('Failed to parse log message:', e)
        }
      }
    }

    // Reset state
    setLines([])
    setError(null)
    setIsConnected(false)

    // Close existing connection
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = null
    }

    // Connect if we have server and token
    if (serverId && token) {
      connect()
    }

    return () => {
      mountedRef.current = false
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current)
      }
      wsRef.current?.close()
    }
  }, [serverId, token])

  return { lines, isConnected, error }
}
