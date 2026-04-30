import { useEffect, useRef, useState, useCallback } from 'react'
import type { WSEvent } from './types'

export function useWebSocket(url: string, onEvent?: (event: WSEvent) => void) {
  const [isConnected, setIsConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimeoutRef = useRef<number | null>(null)
  const onEventRef = useRef(onEvent)
  // Held in a ref so the reconnect setTimeout always invokes the latest
  // closure (e.g. after url changes), not the one captured at construction.
  const connectRef = useRef<() => void>(() => {})

  // Keep callback ref up to date
  useEffect(() => {
    onEventRef.current = onEvent
  }, [onEvent])

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return

    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen = () => {
      setIsConnected(true)
      console.log('WebSocket connected')
    }

    ws.onclose = () => {
      setIsConnected(false)
      console.log('WebSocket disconnected, reconnecting in 3s...')
      reconnectTimeoutRef.current = window.setTimeout(() => connectRef.current(), 3000)
    }

    ws.onerror = (error) => {
      console.error('WebSocket error:', error)
    }

    ws.onmessage = (event) => {
      // Handle multiple JSON messages separated by newlines
      const lines = event.data.split('\n')
      for (const line of lines) {
        if (!line.trim()) continue
        try {
          const data = JSON.parse(line) as WSEvent
          onEventRef.current?.(data)
        } catch (e) {
          console.error('Failed to parse WebSocket message:', e)
        }
      }
    }
  }, [url])

  useEffect(() => {
    connectRef.current = connect
  }, [connect])

  useEffect(() => {
    connect()

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current)
      }
      wsRef.current?.close()
    }
  }, [connect])

  return { isConnected }
}
