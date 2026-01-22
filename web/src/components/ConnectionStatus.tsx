interface ConnectionStatusProps {
  isConnected: boolean
}

export function ConnectionStatus({ isConnected }: ConnectionStatusProps) {
  const statusClass = isConnected ? 'connected' : 'disconnected'
  const statusText = isConnected ? 'Connected' : 'Disconnected'

  return (
    <div
      className={`connection-indicator ${statusClass}`}
      title={statusText}
    />
  )
}
