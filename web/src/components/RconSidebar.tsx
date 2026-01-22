import { useState, useRef, useEffect, FormEvent, KeyboardEvent, useCallback } from 'react'
import type { RconCommand, ServerStatus } from '../types'
import { LogViewer } from './LogViewer'

interface RconSidebarProps {
  server: ServerStatus | null
  token: string
  onClose: () => void
}

const MIN_WIDTH = 300
const MAX_WIDTH = 800
const DEFAULT_WIDTH = 500

type TabType = 'rcon' | 'logs'

export function RconSidebar({ server, token, onClose }: RconSidebarProps) {
  const [activeTab, setActiveTab] = useState<TabType>('rcon')
  const [command, setCommand] = useState('')
  const [history, setHistory] = useState<RconCommand[]>([])
  const [historyIndex, setHistoryIndex] = useState(-1)
  const [rconAvailable, setRconAvailable] = useState<boolean | null>(null)
  const [logAvailable, setLogAvailable] = useState<boolean | null>(null)
  const [width, setWidth] = useState(DEFAULT_WIDTH)
  const [isResizing, setIsResizing] = useState(false)
  const outputRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const commandIdRef = useRef(0)
  const sidebarRef = useRef<HTMLDivElement>(null)

  // Handle resize drag
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    setIsResizing(true)
  }, [])

  useEffect(() => {
    if (!isResizing) return

    const handleMouseMove = (e: MouseEvent) => {
      const newWidth = window.innerWidth - e.clientX
      setWidth(Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, newWidth)))
    }

    const handleMouseUp = () => {
      setIsResizing(false)
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isResizing])

  // Update CSS variable for sidebar width
  useEffect(() => {
    document.documentElement.style.setProperty('--sidebar-width', `${width}px`)
    return () => {
      document.documentElement.style.removeProperty('--sidebar-width')
    }
  }, [width])

  // Check if RCON and logs are available for this server
  useEffect(() => {
    if (!server) {
      setRconAvailable(null)
      setLogAvailable(null)
      return
    }

    fetch(`/api/servers/${server.server_id}/rcon-status`)
      .then(res => res.json())
      .then(data => setRconAvailable(data.available))
      .catch(() => setRconAvailable(false))

    fetch(`/api/servers/${server.server_id}/log-status`, {
      headers: { 'Authorization': `Bearer ${token}` }
    })
      .then(res => res.json())
      .then(data => setLogAvailable(data.available))
      .catch(() => setLogAvailable(false))
  }, [server?.server_id, token])

  // Auto-scroll to bottom when history changes
  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight
    }
  }, [history])

  // Focus input when sidebar opens
  useEffect(() => {
    inputRef.current?.focus()
  }, [server])

  const executeCommand = (cmd: string) => {
    if (!server || !cmd.trim()) return

    const commandId = ++commandIdRef.current
    const newCommand: RconCommand = {
      id: commandId,
      command: cmd,
      output: '...',
      timestamp: new Date(),
      serverName: server.name,
    }

    // Immediately update UI
    setHistory(prev => [...prev, newCommand])
    setCommand('')
    setHistoryIndex(-1)
    inputRef.current?.focus()

    // Fetch response asynchronously
    fetch(`/api/servers/${server.server_id}/rcon`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify({ command: cmd }),
    })
      .then(async res => {
        if (!res.ok) {
          const error = await res.json()
          return `Error: ${error.error}`
        }
        const data = await res.json()
        return data.output || '(no output)'
      })
      .catch(err => `Error: ${err}`)
      .then(output => {
        setHistory(prev =>
          prev.map(h => (h.id === commandId ? { ...h, output } : h))
        )
      })
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    executeCommand(command)
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      const commands = history.filter(h => h.serverName === server?.name)
      if (commands.length === 0) return
      const newIndex = historyIndex < commands.length - 1 ? historyIndex + 1 : historyIndex
      setHistoryIndex(newIndex)
      setCommand(commands[commands.length - 1 - newIndex]?.command || '')
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (historyIndex > 0) {
        const commands = history.filter(h => h.serverName === server?.name)
        const newIndex = historyIndex - 1
        setHistoryIndex(newIndex)
        setCommand(commands[commands.length - 1 - newIndex]?.command || '')
      } else {
        setHistoryIndex(-1)
        setCommand('')
      }
    }
  }

  const renderTabContent = () => {
    if (!server) {
      return (
        <div className="sidebar-placeholder">
          Select a server
        </div>
      )
    }

    if (activeTab === 'logs') {
      if (logAvailable === false) {
        return (
          <div className="sidebar-unavailable">
            Logs are not configured for this server
          </div>
        )
      }
      return <LogViewer server={server} token={token} />
    }

    // RCON tab
    if (rconAvailable === false) {
      return (
        <div className="sidebar-unavailable">
          RCON is not configured for this server
        </div>
      )
    }

    return (
      <>
        <div className="rcon-output" ref={outputRef}>
          {history.filter(h => h.serverName === server.name).map(cmd => (
            <div key={cmd.id} className="rcon-entry">
              <div className="rcon-command">&gt; {cmd.command}</div>
              <pre className="rcon-response">{cmd.output}</pre>
            </div>
          ))}
        </div>

        <form onSubmit={handleSubmit} className="rcon-input-form">
          <input
            ref={inputRef}
            type="text"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Enter command..."
            disabled={rconAvailable === null}
            autoComplete="off"
            spellCheck={false}
          />
          <button type="submit" disabled={!command.trim()}>
            Send
          </button>
        </form>
      </>
    )
  }

  return (
    <div className={`rcon-sidebar ${isResizing ? 'resizing' : ''}`} style={{ width }} ref={sidebarRef}>
      <div className="resize-handle" onMouseDown={handleMouseDown} />
      <div className="rcon-header">
        <h3>{server ? server.name : 'Admin'}</h3>
        <button onClick={onClose} className="close-btn">X</button>
      </div>

      <div className="sidebar-tabs">
        <button
          className={`sidebar-tab ${activeTab === 'rcon' ? 'active' : ''}`}
          onClick={() => setActiveTab('rcon')}
        >
          RCON
        </button>
        <button
          className={`sidebar-tab ${activeTab === 'logs' ? 'active' : ''}`}
          onClick={() => setActiveTab('logs')}
        >
          Logs
        </button>
      </div>

      <div className="sidebar-content">
        {renderTabContent()}
      </div>
    </div>
  )
}
