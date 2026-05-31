import { useEffect, useRef, useState, useCallback } from "react";
import type { WSEvent } from "./types";

export interface WebSocketHandle {
  isConnected: boolean;
  send: (payload: object) => void;
}

export function useWebSocket(
  url: string,
  onEvent?: (event: WSEvent) => void,
): WebSocketHandle {
  const [isConnected, setIsConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);
  const onEventRef = useRef(onEvent);
  // Cache the most recent subscribe payload so we can replay it after a
  // reconnect — otherwise the server would revert the new socket to
  // its default {server, activity} subs and the drawer-closed client
  // would silently start receiving activity events again.
  const lastSubscribeRef = useRef<object | null>(null);
  // Held in a ref so the reconnect setTimeout always invokes the latest
  // closure (e.g. after url changes), not the one captured at construction.
  const connectRef = useRef<() => void>(() => {});

  // Keep callback ref up to date
  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setIsConnected(true);
      console.log("WebSocket connected");
      // Replay the most recent subscribe so server-side subs reflect the
      // client's current intent (drawer open vs closed).
      if (lastSubscribeRef.current) {
        try {
          ws.send(JSON.stringify(lastSubscribeRef.current));
        } catch {
          /* ignore */
        }
      }
    };

    ws.onclose = () => {
      setIsConnected(false);
      console.log("WebSocket disconnected, reconnecting in 3s...");
      reconnectTimeoutRef.current = window.setTimeout(
        () => connectRef.current(),
        3000,
      );
    };

    ws.onerror = (error) => {
      console.error("WebSocket error:", error);
    };

    ws.onmessage = (event) => {
      // Handle multiple JSON messages separated by newlines
      const lines = event.data.split("\n");
      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          const data = JSON.parse(line) as WSEvent;
          onEventRef.current?.(data);
        } catch (e) {
          console.error("Failed to parse WebSocket message:", e);
        }
      }
    };
  }, [url]);

  useEffect(() => {
    connectRef.current = connect;
  }, [connect]);

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      wsRef.current?.close();
    };
  }, [connect]);

  const send = useCallback((payload: object) => {
    // Cache subscribe payloads so reconnects can replay them.
    if ((payload as { type?: string }).type === "subscribe") {
      lastSubscribeRef.current = payload;
    }
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      try {
        ws.send(JSON.stringify(payload));
      } catch {
        /* ignore */
      }
    }
    // If the socket isn't open yet, the cached payload above will be
    // replayed in onopen; no queueing needed.
  }, []);

  return { isConnected, send };
}
