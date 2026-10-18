import {
  useState,
  useRef,
  useEffect,
  useMemo,
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { ColoredText } from "./ColoredText";
import { apiFetch } from "../authFetch";
import { CloseIcon } from "./CloseIcon";
import type { RconCommand, ServerStatus } from "../types";

interface RconSidebarProps {
  server: ServerStatus | null;
  onClose: () => void;
}

const HISTORY_CAP = 200;

// Shares the .drawer chrome with ActivityDrawer and MyServersDrawer;
// mounting it opens it.
export function RconSidebar({ server, onClose }: RconSidebarProps) {
  const [command, setCommand] = useState("");
  const [history, setHistory] = useState<RconCommand[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [rconAvailable, setRconAvailable] = useState<boolean | null>(null);
  const outputRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const commandIdRef = useRef(0);
  // Aborted on unmount so a late response can't land in the next mount.
  const inflightRef = useRef<Set<AbortController>>(new Set());

  // Esc closes; document-level so focus inside the drawer still reaches it.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    const inflight = inflightRef.current;
    return () => {
      for (const ctrl of inflight) ctrl.abort();
      inflight.clear();
    };
  }, []);

  // Keyed on server_id so other fields ticking (game_time_ms) don't refetch.
  useEffect(() => {
    if (!server) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setRconAvailable(null);
      return;
    }
    const ctrl = new AbortController();
    apiFetch(`/api/servers/${server.server_id}/rcon-status`, {
      signal: ctrl.signal,
    })
      .then((res) => res.json())
      .then((data) => setRconAvailable(data.available))
      .catch(() => {
        if (!ctrl.signal.aborted) setRconAvailable(false);
      });
    return () => ctrl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [server?.server_id]);

  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [history]);

  useEffect(() => {
    inputRef.current?.focus();
  }, [server]);

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
      output: "…",
      timestamp: new Date(),
      serverName: server.key,
    };

    setHistory((prev) => [...prev, newCommand].slice(-HISTORY_CAP));
    setCommand("");
    setHistoryIndex(-1);
    inputRef.current?.focus();

    const ctrl = new AbortController();
    inflightRef.current.add(ctrl);
    apiFetch(`/api/servers/${server.server_id}/rcon`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
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
          Pick a server to send commands to.
        </div>
      );
    }

    if (rconAvailable === false) {
      return (
        <div className="rcon-drawer__unavailable">
          RCON isn&rsquo;t set up for this server.
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
                : "Type a command below. ↑ and ↓ recall earlier ones."}
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
