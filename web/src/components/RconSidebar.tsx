import {
  useState,
  useRef,
  useEffect,
  useMemo,
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { ColoredText } from "./ColoredText";
import { CloseIcon } from "./CloseIcon";
import type { RconCommand, ServerStatus } from "../types";

interface RconSidebarProps {
  server: ServerStatus | null;
  token: string;
  onClose: () => void;
}

// Cap on per-mount command history. Long sessions running `dumpuser`
// or repeated `status` calls would grow unbounded otherwise.
const HISTORY_CAP = 200;

// Right-side overlay drawer for RCON command execution. Shares the
// .drawer chrome with ActivityDrawer / MyServersDrawer; mount/unmount
// is the open/close gesture. Esc + backdrop both close.
export function RconSidebar({ server, token, onClose }: RconSidebarProps) {
  const [command, setCommand] = useState("");
  const [history, setHistory] = useState<RconCommand[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [rconAvailable, setRconAvailable] = useState<boolean | null>(null);
  const outputRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const commandIdRef = useRef(0);
  // Tracks every in-flight RCON command fetch so they can be aborted
  // on unmount (preventing setState on a dead component and dropping
  // late responses that would otherwise land in the next mount).
  const inflightRef = useRef<Set<AbortController>>(new Set());

  // Esc closes; document-level so focus inside the drawer still reaches it.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Abort every in-flight command fetch on unmount.
  useEffect(() => {
    const inflight = inflightRef.current;
    return () => {
      for (const ctrl of inflight) ctrl.abort();
      inflight.clear();
    };
  }, []);

  // Check if RCON is available for this server. Keying on server_id only
  // so we don't re-fetch when other server fields tick (game_time_ms, etc.)
  useEffect(() => {
    if (!server) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setRconAvailable(null);
      return;
    }
    const ctrl = new AbortController();
    fetch(`/api/servers/${server.server_id}/rcon-status`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then((res) => res.json())
      .then((data) => setRconAvailable(data.available))
      .catch(() => {
        // Genuine network error → RCON unreachable. Aborts also land
        // here but the unmounted setter is a no-op, so we don't gate.
        if (!ctrl.signal.aborted) setRconAvailable(false);
      });
    return () => ctrl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [server?.server_id, token]);

  // Auto-scroll to bottom when history changes
  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [history]);

  // Focus input when sidebar opens
  useEffect(() => {
    inputRef.current?.focus();
  }, [server]);

  // Per-server view of history — used by both arrow-key recall and the
  // output list. Recomputing once per render beats four times.
  const serverHistory = useMemo(
    () => history.filter((h) => h.serverName === server?.key),
    [history, server?.key],
  );

  const executeCommand = (cmd: string) => {
    if (!server || !cmd.trim()) return;

    const commandId = ++commandIdRef.current;
    const newCommand: RconCommand = {
      id: commandId,
      command: cmd,
      output: "...",
      timestamp: new Date(),
      serverName: server.key,
    };

    // Immediately update UI; cap at HISTORY_CAP entries so a long
    // session doesn't grow unbounded.
    setHistory((prev) => [...prev, newCommand].slice(-HISTORY_CAP));
    setCommand("");
    setHistoryIndex(-1);
    inputRef.current?.focus();

    const ctrl = new AbortController();
    inflightRef.current.add(ctrl);
    fetch(`/api/servers/${server.server_id}/rcon`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ command: cmd }),
      signal: ctrl.signal,
    })
      .then(async (res) => {
        if (!res.ok) {
          const error = await res.json();
          return `Error: ${error.error}`;
        }
        const data = await res.json();
        return data.output || "(no output)";
      })
      .catch((err) => (ctrl.signal.aborted ? null : `Error: ${err}`))
      .then((output) => {
        inflightRef.current.delete(ctrl);
        if (output === null) return; // aborted on unmount
        setHistory((prev) =>
          prev.map((h) => (h.id === commandId ? { ...h, output } : h)),
        );
      });
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    executeCommand(command);
  };

  const handleKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowUp") {
      e.preventDefault();
      if (serverHistory.length === 0) return;
      const newIndex =
        historyIndex < serverHistory.length - 1
          ? historyIndex + 1
          : historyIndex;
      setHistoryIndex(newIndex);
      setCommand(
        serverHistory[serverHistory.length - 1 - newIndex]?.command || "",
      );
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      if (historyIndex > 0) {
        const newIndex = historyIndex - 1;
        setHistoryIndex(newIndex);
        setCommand(
          serverHistory[serverHistory.length - 1 - newIndex]?.command || "",
        );
      } else {
        setHistoryIndex(-1);
        setCommand("");
      }
    }
  };

  const renderBody = () => {
    if (!server) {
      return (
        <div className="rcon-drawer__placeholder">
          Select a server to issue commands.
        </div>
      );
    }

    if (rconAvailable === false) {
      return (
        <div className="rcon-drawer__unavailable">
          RCON is not configured for this server.
        </div>
      );
    }

    return (
      <>
        <div className="rcon-drawer__output" ref={outputRef}>
          {serverHistory.length === 0 ? (
            <div className="rcon-drawer__hint">
              {rconAvailable === null
                ? "Checking RCON…"
                : "Issue your first command below. ↑/↓ recalls history."}
            </div>
          ) : (
            serverHistory.map((cmd) => (
              <div key={cmd.id} className="rcon-drawer__entry">
                <div className="rcon-drawer__command">&gt; {cmd.command}</div>
                <pre className="rcon-drawer__response">{cmd.output}</pre>
              </div>
            ))
          )}
        </div>

        <form onSubmit={handleSubmit} className="rcon-drawer__form">
          <input
            ref={inputRef}
            type="text"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="rcon command…"
            disabled={rconAvailable === null}
            autoComplete="off"
            spellCheck={false}
            aria-label="RCON command"
          />
          <button type="submit" disabled={!command.trim()}>
            Send
          </button>
        </form>
      </>
    );
  };

  return (
    <div className="drawer-overlay" onClick={onClose}>
      <aside
        className="drawer rcon-drawer"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-label="RCON console"
      >
        <div className="drawer-header">
          <h2>
            RCON
            {server && (
              <>
                <span className="rcon-drawer__sep" aria-hidden="true">
                  ·
                </span>
                <span className="rcon-drawer__server">
                  <ColoredText text={server.key} />
                </span>
              </>
            )}
          </h2>
          <button onClick={onClose} className="close-btn" aria-label="Close">
            <CloseIcon />
          </button>
        </div>

        <div className="drawer-body rcon-drawer__body">{renderBody()}</div>
      </aside>
    </div>
  );
}
