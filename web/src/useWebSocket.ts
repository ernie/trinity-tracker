import { useEffect, useRef, useState, useCallback } from "react";
import type { WSEvent } from "./types";

// "connecting": the first load, or a drop expected to recover (a background
//   tab's socket closed for idling, the first visible retries).
// "open": the socket is up.
// "down": the browser reports no network, or 3 visible reconnects in a row
//   failed. Retrying goes on; this only sets the OFFLINE label.
export type ConnectionStatus = "connecting" | "open" | "down";

const OFFLINE_AFTER_FAILURES = 3;

export interface WebSocketHandle {
  status: ConnectionStatus;
  isConnected: boolean;
  send: (payload: object) => void;
}

export function useWebSocket(
  url: string,
  onEvent?: (event: WSEvent) => void,
): WebSocketHandle {
  const [status, setStatus] = useState<ConnectionStatus>("connecting");
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);
  // Visible-tab closes since the last open.
  const failuresRef = useRef(0);
  const onEventRef = useRef(onEvent);
  // Sent again on every open: a new socket starts on the server's default
  // subscriptions, which include activity events a closed drawer doesn't want.
  const lastSubscribeRef = useRef<object | null>(null);
  // In a ref so the reconnect timer calls the current closure after url changes.
  const connectRef = useRef<() => void>(() => {});

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
  }, []);

  const connect = useCallback(() => {
    const state = wsRef.current?.readyState;
    if (state === WebSocket.OPEN || state === WebSocket.CONNECTING) return;

    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      failuresRef.current = 0;
      setStatus("open");
      console.log("WebSocket connected");
      if (lastSubscribeRef.current) {
        try {
          ws.send(JSON.stringify(lastSubscribeRef.current));
        } catch {
          /* ignore */
        }
      }
    };

    ws.onclose = () => {
      if (document.hidden) {
        // Browsers close idle sockets in background tabs. The
        // visibilitychange handler reconnects when the tab returns.
        setStatus((s) => (s === "down" ? s : "connecting"));
        console.log("WebSocket closed in background, waiting for tab focus");
        return;
      }
      failuresRef.current += 1;
      setStatus(
        failuresRef.current >= OFFLINE_AFTER_FAILURES ? "down" : "connecting",
      );
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
      // One message can carry several newline-separated JSON events.
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
      clearReconnectTimer();
      wsRef.current?.close();
    };
  }, [connect, clearReconnectTimer]);

  useEffect(() => {
    const onVisibilityChange = () => {
      if (document.visibilityState !== "visible") return;
      const state = wsRef.current?.readyState;
      if (state === WebSocket.OPEN || state === WebSocket.CONNECTING) return;
      clearReconnectTimer();
      connectRef.current();
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () =>
      document.removeEventListener("visibilitychange", onVisibilityChange);
  }, [clearReconnectTimer]);

  // The browser's online and offline events outrank the failure count.
  useEffect(() => {
    const onOffline = () => setStatus("down");
    const onOnline = () => {
      failuresRef.current = 0;
      const state = wsRef.current?.readyState;
      if (state === WebSocket.OPEN || state === WebSocket.CONNECTING) return;
      setStatus("connecting");
      clearReconnectTimer();
      connectRef.current();
    };
    window.addEventListener("offline", onOffline);
    window.addEventListener("online", onOnline);
    return () => {
      window.removeEventListener("offline", onOffline);
      window.removeEventListener("online", onOnline);
    };
  }, [clearReconnectTimer]);

  const send = useCallback((payload: object) => {
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
  }, []);

  return { status, isConnected: status === "open", send };
}
