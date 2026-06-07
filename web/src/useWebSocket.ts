import { useEffect, useRef, useState, useCallback } from "react";
import type { WSEvent } from "./types";

// "connecting" — initial load, or a drop we expect to recover from
//   (idle-killed background tab, first couple of visible retries).
// "open" — socket is up.
// "down" — the network is known-offline, or 3 consecutive visible
//   reconnect attempts failed. Still retrying; this only drives honest
//   OFFLINE labeling, not behavior.
export type ConnectionStatus = "connecting" | "open" | "down";

// Visible-tab close count before we admit the hub is unreachable.
const OFFLINE_AFTER_FAILURES = 3;

export interface WebSocketHandle {
  status: ConnectionStatus;
  /** Convenience: status === "open". */
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
  // Consecutive visible-tab closes without an intervening open.
  const failuresRef = useRef(0);
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
      if (document.hidden) {
        // Idle/throttle kill in a background tab — not a failure. Park
        // (timers are throttled while hidden anyway); the visibilitychange
        // handler reconnects the moment the tab returns.
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
      clearReconnectTimer();
      wsRef.current?.close();
    };
  }, [connect, clearReconnectTimer]);

  // Returning tab: reconnect immediately instead of waiting out (or having
  // parked) the retry timer. Counts as a fresh start, not a failure streak.
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

  // The browser knows when the network itself is gone — trust it both ways.
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

  return { status, isConnected: status === "open", send };
}
