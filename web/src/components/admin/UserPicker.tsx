import { useEffect, useState } from "react";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";
import { apiFetch } from "../../authFetch";
import { ColoredText } from "../ColoredText";

export interface UserOption {
  id: number;
  username: string;
  is_admin: boolean;
  player_name?: string | null;
}

interface Props {
  selected: UserOption | null;
  onChange: (user: UserOption | null) => void;
  placeholder?: string;
  // Shown but unselectable, such as the current owner in a transfer.
  excludeUserId?: number;
  autoFocus?: boolean;
  required?: boolean;
}

export function UserPicker({
  selected,
  onChange,
  placeholder = "Search users…",
  excludeUserId,
  autoFocus,
  required,
}: Props) {
  const [query, setQuery] = useState(selected?.username ?? "");
  const [results, setResults] = useState<UserOption[]>([]);
  const debounced = useDebouncedValue(query.trim(), 200);

  // A query equal to the picked user's name is the pick itself, so it
  // neither searches nor shows results; the dropdown stays closed.
  useEffect(() => {
    if (debounced.length < 2) return;
    if (
      selected &&
      debounced.toLowerCase() === selected.username.toLowerCase()
    ) {
      return;
    }
    const ctrl = new AbortController();
    apiFetch(`/api/users?search=${encodeURIComponent(debounced)}&limit=10`, {
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((users: UserOption[]) => setResults(users ?? []))
      .catch(() => {
        /* keep the previous results */
      });
    return () => ctrl.abort();
  }, [debounced, selected]);

  const showResults =
    debounced.length >= 2 &&
    !(selected && debounced.toLowerCase() === selected.username.toLowerCase());
  const displayResults = showResults ? results : [];

  const pick = (u: UserOption) => {
    onChange(u);
    setQuery(u.username);
    setResults([]);
  };

  const clear = () => {
    onChange(null);
    setQuery("");
    setResults([]);
  };

  return (
    <div className="user-picker">
      <input
        type="text"
        placeholder={placeholder}
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          if (selected && e.target.value !== selected.username) {
            onChange(null);
          }
        }}
        autoFocus={autoFocus}
        required={required && !selected}
        autoComplete="off"
        spellCheck={false}
      />
      {displayResults.length > 0 && (
        <ul className="player-results user-picker-results">
          {displayResults.map((u) => {
            const disabled =
              excludeUserId !== undefined && u.id === excludeUserId;
            return (
              <li
                key={u.id}
                className={disabled ? "user-picker-disabled" : ""}
                onClick={() => {
                  if (!disabled) pick(u);
                }}
              >
                <span className="user-picker-username">
                  {u.username}
                  {u.is_admin && (
                    <span className="user-picker-badge">admin</span>
                  )}
                </span>
                {u.player_name && (
                  <span className="user-picker-player">
                    (<ColoredText text={u.player_name} />)
                  </span>
                )}
                {disabled && (
                  <span className="user-picker-note">current owner</span>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {selected && (
        <div className="selected-player">
          <span>
            {selected.username}
            {selected.is_admin && " (admin)"}
            {selected.player_name && (
              <>
                {" ("}
                <ColoredText text={selected.player_name} />
                {")"}
              </>
            )}
          </span>
          <button type="button" onClick={clear}>
            Clear
          </button>
        </div>
      )}
    </div>
  );
}
