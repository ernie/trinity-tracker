import { ReactNode, useEffect, useState } from 'react'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'

export interface UserOption {
  id: number
  username: string
  is_admin: boolean
  player_name?: string | null
}

interface Props {
  token: string
  selected: UserOption | null
  onChange: (user: UserOption | null) => void
  placeholder?: string
  // excludeUserId disables the row in results (used for transfer-owner where
  // the current owner shouldn't be re-selectable).
  excludeUserId?: number
  autoFocus?: boolean
  required?: boolean
}

export function UserPicker({
  token,
  selected,
  onChange,
  placeholder = 'Search users…',
  excludeUserId,
  autoFocus,
  required,
}: Props) {
  const [query, setQuery] = useState(selected?.username ?? '')
  const [results, setResults] = useState<UserOption[]>([])
  const debounced = useDebouncedValue(query.trim(), 200)

  // Fire a search whenever the debounced query changes. Skip when the query
  // already matches the picked user's username — otherwise selecting a row
  // would re-trigger a search and re-open the dropdown a moment later.
  useEffect(() => {
    if (debounced.length < 2) {
      setResults([])
      return
    }
    if (selected && debounced.toLowerCase() === selected.username.toLowerCase()) {
      return
    }
    const ctrl = new AbortController()
    fetch(`/api/users?search=${encodeURIComponent(debounced)}&limit=10`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((users: UserOption[]) => setResults(users ?? []))
      .catch(() => {
        /* aborted or network error — leave previous results in place */
      })
    return () => ctrl.abort()
  }, [debounced, token, selected])

  const pick = (u: UserOption) => {
    onChange(u)
    setQuery(u.username)
    setResults([])
  }

  const clear = () => {
    onChange(null)
    setQuery('')
    setResults([])
  }

  return (
    <div className="user-picker">
      <input
        type="text"
        placeholder={placeholder}
        value={query}
        onChange={(e) => {
          setQuery(e.target.value)
          if (selected && e.target.value !== selected.username) {
            onChange(null)
          }
        }}
        autoFocus={autoFocus}
        required={required && !selected}
        autoComplete="off"
        spellCheck={false}
      />
      {results.length > 0 && (
        <ul className="player-results user-picker-results">
          {results.map((u) => {
            const disabled = excludeUserId !== undefined && u.id === excludeUserId
            return (
              <li
                key={u.id}
                className={disabled ? 'user-picker-disabled' : ''}
                onClick={() => {
                  if (!disabled) pick(u)
                }}
              >
                <span className="user-picker-username">
                  {highlight(u.username, debounced)}
                  {u.is_admin && <span className="user-picker-badge">admin</span>}
                </span>
                {u.player_name && (
                  <span className="user-picker-player">
                    player: {highlight(u.player_name, debounced)}
                  </span>
                )}
                {disabled && <span className="user-picker-note">current owner</span>}
              </li>
            )
          })}
        </ul>
      )}
      {selected && (
        <div className="selected-player">
          <span>
            Selected: {selected.username}
            {selected.is_admin && ' (admin)'}
            {selected.player_name && ` · player: ${selected.player_name}`}
          </span>
          <button type="button" onClick={clear}>
            Clear
          </button>
        </div>
      )}
    </div>
  )
}

// highlight wraps every case-insensitive occurrence of `q` in `s` with <mark>
// so the admin can see at a glance which field (username vs. player name) the
// query matched on.
function highlight(s: string, q: string): ReactNode {
  if (!q) return s
  const needle = q.toLowerCase()
  const lower = s.toLowerCase()
  const out: ReactNode[] = []
  let i = 0
  let key = 0
  while (i < s.length) {
    const idx = lower.indexOf(needle, i)
    if (idx < 0) {
      out.push(s.slice(i))
      break
    }
    if (idx > i) out.push(s.slice(i, idx))
    out.push(<mark key={key++}>{s.slice(idx, idx + needle.length)}</mark>)
    i = idx + needle.length
  }
  return out
}
