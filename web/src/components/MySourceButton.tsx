import { useState } from 'react'
import { useMySources } from '../hooks/useMySource'
import { MyServersDrawer } from './MyServersDrawer'

// MySourceButton is the contextual header button driven by
// /api/sources/mine. It picks one of three labels and opens the
// always-on drawer; the drawer renders both the source list and the
// request form, so there's a single surface for everything.
//
// Label rules:
//   no rows                                → "Add Servers"
//   any pending                            → "Request Pending"
//   any active (no pending)                → "My Servers" or "My Servers (N)"
//   only rejected/left/revoked rows        → "Add Servers"
export function MySourceButton() {
  const { data, refresh } = useMySources()
  const [open, setOpen] = useState(false)

  const actives = data.sources.filter((s) => s.status === 'active')

  let label = 'Add Servers'
  let cls = 'my-source-btn'
  if (data.has_pending) {
    label = 'Request Pending'
    cls += ' my-source-btn-pending'
  } else if (actives.length > 0) {
    label =
      actives.length === 1 ? 'My Servers' : `My Servers (${actives.length})`
    cls += ' my-source-btn-active'
  }

  return (
    <>
      <button className={cls} onClick={() => setOpen(true)}>
        {label}
      </button>
      {open && (
        <MyServersDrawer
          sources={data.sources}
          hasPending={data.has_pending}
          onClose={() => setOpen(false)}
          onRefresh={refresh}
        />
      )}
    </>
  )
}
