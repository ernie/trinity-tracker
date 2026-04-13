import { useEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { Link, Navigate } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'

export function PlayPage() {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const statusRef = useRef<HTMLDivElement>(null)
  const moduleRef = useRef<any>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [progress, setProgress] = useState({ loaded: 0, total: 0 })
  const [, setEngineReady] = useState(false)
  const { auth, loading: authLoading } = useAuth()

  useEffect(() => {
    if (authLoading || !auth.isAdmin) return
    let aborted = false

    async function init() {
      try {
        // @ts-ignore — runtime module from WASM engine, not in TS source tree
        const { loadEngine } = await import(/* @vite-ignore */ '/engine/loader.js')
        if (aborted) return

        const rect = canvasRef.current!.getBoundingClientRect()
        const dpr = window.devicePixelRatio || 1
        const mod = await loadEngine({
          canvas: canvasRef.current!,
          statusEl: statusRef.current!,
          enginePath: '/engine/',
          configUrl: '/engine/client-config.json',
          authToken: auth.token,
          extraArgs: `+set r_mode -1 +set r_customwidth ${Math.round(rect.width * dpr)} +set r_customheight ${Math.round(rect.height * dpr)}`,
          onProgress: (loaded: number, total: number) => setProgress({ loaded, total }),
          onReady: () => {
            setEngineReady(true)
            if (statusRef.current) statusRef.current.style.display = 'none'
          },
        })
        moduleRef.current = mod
        if (aborted) {
          try { mod.abort(); } catch {}
          return
        }
        setLoading(false)
      } catch (e: any) {
        if (!aborted) setError(e.message || 'Failed to load engine')
      }
    }

    init()

    return () => {
      aborted = true
      const mod = moduleRef.current
      if (mod) {
        try { mod.shutdown(); } catch {}
        try { mod.pauseMainLoop(); } catch {}
        try { mod._exit(0); } catch {}
        moduleRef.current = null
      }
    }
  }, [auth.isAdmin, auth.token, authLoading])

  // Re-initialize video on resize so the framebuffer matches the CSS box
  useEffect(() => {
    const canvas = canvasRef.current
    const mod = moduleRef.current
    if (!canvas || !mod) return
    let timer: ReturnType<typeof setTimeout> | null = null
    const dpr = window.devicePixelRatio || 1
    let initW = Math.round(canvas.getBoundingClientRect().width * dpr)
    let initH = Math.round(canvas.getBoundingClientRect().height * dpr)
    const observer = new ResizeObserver(() => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        const rect = canvas.getBoundingClientRect()
        const w = Math.round(rect.width * dpr)
        const h = Math.round(rect.height * dpr)
        if (w === initW && h === initH) return
        initW = w
        initH = h
        flushSync(() => setEngineReady(false))
        canvas.style.visibility = 'hidden'
        if (statusRef.current) {
          statusRef.current.style.display = ''
          statusRef.current.textContent = 'Restarting video...'
        }
        requestAnimationFrame(() => {
          mod.ccall('Cbuf_AddText', null, ['string'],
            [`r_mode -1\nr_customwidth ${w}\nr_customheight ${h}\nvid_restart\n`])
          mod.onNextFrame?.(() => {
            canvas.style.visibility = ''
            setEngineReady(true)
            if (statusRef.current) statusRef.current.style.display = 'none'
          })
        })
      }, 200)
    })
    observer.observe(canvas)
    return () => { observer.disconnect(); if (timer) clearTimeout(timer) }
  }, [loading])

  useEffect(() => {
    const handler = () => {
      canvasRef.current?.classList.toggle('no-pointerlock', !document.pointerLockElement)
    }
    document.addEventListener('pointerlockchange', handler)
    return () => document.removeEventListener('pointerlockchange', handler)
  }, [])

  // Prevent Tab key from leaving the canvas
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Tab') e.preventDefault()
    }
    window.addEventListener('keydown', handler, true)
    return () => window.removeEventListener('keydown', handler, true)
  }, [])

  if (authLoading) {
    return <div className="demo-player-page"><div className="demo-status">Loading...</div></div>
  }

  if (!auth.isAuthenticated || !auth.isAdmin) {
    return <Navigate to="/" replace />
  }

  if (error) {
    return (
      <div className="demo-player-page">
        <div className="demo-player-error">
          <p>{error}</p>
          <Link to="/">Back to home</Link>
        </div>
      </div>
    )
  }

  return (
    <div className="demo-player-page">
      <Link to="/" className="demo-back-link">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <polyline points="15,18 9,12 15,6" />
        </svg>
        Back
      </Link>
      <canvas ref={canvasRef} id="canvas" tabIndex={0} className="demo-canvas" />
      <div ref={statusRef} className="demo-status">{loading ? 'Loading...' : ''}</div>
      {progress.total > 0 && (
        <div className="demo-progress" style={{ opacity: progress.loaded >= progress.total ? 0 : 1 }}>
          <div className="demo-progress-bar" style={{ width: `${Math.min(100, (progress.loaded / progress.total) * 100)}%` }} />
        </div>
      )}
    </div>
  )
}
