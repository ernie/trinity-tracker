import { useLocation } from 'react-router-dom'
import { Header } from './Header'

// Route-aware Header. Mounted once at the shell so its DOM persists
// across navigations — the right-side community / auth cluster no
// longer flashes through a remount when nav tabs change. Title is
// derived from pathname; routes with their own header (landing,
// demo player, play canvas) render nothing.
function deriveTitle(pathname: string): { title: string; wordmark?: string } | null {
  if (pathname === '/') return null
  if (pathname === '/play') return null
  if (/^\/matches\/\d+\/demo$/.test(pathname)) return null

  if (/^\/matches\/\d+/.test(pathname)) return { title: 'Match Details' }
  if (pathname.startsWith('/matches')) return { title: 'Matches' }
  if (pathname.startsWith('/players')) return { title: 'Players' }
  if (pathname.startsWith('/leaderboard')) return { title: 'Leaderboard' }
  if (pathname.startsWith('/docs')) return { title: 'Docs' }
  if (pathname.startsWith('/account')) return { title: 'My Account' }
  if (pathname.startsWith('/admin')) return { title: 'Admin' }
  if (pathname.startsWith('/claim')) return { title: 'Claim Identity' }
  if (pathname.startsWith('/quake3-eula')) return { title: 'Quake 3 EULA' }
  return { title: 'Trinity', wordmark: 'tracker' }
}

export function RouteHeader() {
  const { pathname } = useLocation()
  const meta = deriveTitle(pathname)
  if (!meta) return null
  return <Header title={meta.title} className="app-header" wordmark={meta.wordmark} />
}
