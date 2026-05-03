import { useEffect, useState } from 'react'

// useDebouncedValue returns `value` after it has stayed unchanged for `delay`
// milliseconds. Suitable for typeahead search inputs that should only fire a
// fetch once the user pauses typing.
export function useDebouncedValue<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(id)
  }, [value, delay])
  return debounced
}
