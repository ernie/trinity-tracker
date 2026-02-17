import { useEffect, useRef, useState, useCallback } from 'react'
import { flushSync } from 'react-dom'
import { useParams, Link } from 'react-router-dom'
import { ColoredText } from './ColoredText'
import { PlayerPortrait } from './PlayerPortrait'

interface MatchData {
  id: number
  map_name: string
  demo_url?: string
}

export function DemoPlayerPage() {
  const { id } = useParams<{ id: string }>()
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const statusRef = useRef<HTMLDivElement>(null)
  const moduleRef = useRef<any>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [progress, setProgress] = useState({ loaded: 0, total: 0 })
  const [mapName, setMapName] = useState<string | null>(null)
  const [engineReady, setEngineReady] = useState(false)
  const [scrubActive, setScrubActive] = useState(false)
  const scrubRef = useRef(false)
  const [playerList, setPlayerList] = useState<{ clientNum: number; name: string; team: number; model: string; isVR: boolean }[]>([])
  const [viewpoint, setViewpoint] = useState(-1)
  const [playerListOpen, setPlayerListOpen] = useState(false)
  const playerListOpenRef = useRef(false)
  const playerWrapRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let aborted = false

    async function init() {
      try {
        const resp = await fetch(`/api/matches/${id}`)
        if (!resp.ok) {
          setError(`Match not found (${resp.status})`)
          return
        }
        const match: MatchData = await resp.json()
        setMapName(match.map_name)
        if (!match.demo_url) {
          setError('No demo available for this match')
          return
        }
        if (aborted) return

        // @ts-ignore — runtime module from WASM engine, not in TS source tree
        const { loadDemo } = await import(/* @vite-ignore */ '/demo/demo-loader.js')
        if (aborted) return

        const rect = canvasRef.current!.getBoundingClientRect()
        const dpr = window.devicePixelRatio || 1
        const mod = await loadDemo({
          canvas: canvasRef.current!,
          statusEl: statusRef.current!,
          enginePath: '/demo/',
          demoUrl: match.demo_url,
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
        if (!aborted) setError(e.message || 'Failed to load demo')
      }
    }

    init()

    return () => {
      aborted = true
      const mod = moduleRef.current
      if (mod) {
        // Close audio immediately to avoid looping the last buffer fragment
        try {
          const sdl2 = mod.SDL2
          if (sdl2?.audio?.scriptProcessorNode) sdl2.audio.scriptProcessorNode.disconnect()
          if (sdl2?.audioContext) sdl2.audioContext.close()
        } catch {}
        // pauseMainLoop decrements the keepalive counter so _exit can shut down
        try { mod.pauseMainLoop(); } catch {}
        try { mod._exit(0); } catch {}
        moduleRef.current = null
      }
    }
  }, [id])

  // On mobile, intercept touch events before Emscripten's SDL2 handlers so
  // touches produce mouse motion (camera rotation) but not clicks (follow next).
  // Must register before loadDemo so our capture-phase handlers run first.
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    let lastX = 0, lastY = 0
    // Synthetic mouse position: SDL computes deltas from absolute coords,
    // so we track a virtual cursor that only moves by touch deltas to avoid
    // a camera jump when first touching the screen.
    const rect = canvas.getBoundingClientRect()
    let synthX = rect.left + rect.width / 2
    let synthY = rect.top + rect.height / 2
    let pinchDist = 0
    const PINCH_STEP = 30 // pixels of pinch distance per zoom step
    const pinchLen = (t: TouchList) => {
      const dx = t[0].clientX - t[1].clientX
      const dy = t[0].clientY - t[1].clientY
      return Math.sqrt(dx * dx + dy * dy)
    }
    const onStart = (e: TouchEvent) => {
      e.stopImmediatePropagation()
      e.preventDefault()
      if (playerListOpenRef.current) setPlayerListOpen(false)
      const ct = e.targetTouches
      if (ct.length >= 2) {
        pinchDist = pinchLen(ct)
      } else if (ct.length === 1) {
        lastX = ct[0].clientX
        lastY = ct[0].clientY
        // Reset synthetic mouse to the touch point so each swipe gets a
        // fresh start.  Briefly zero sensitivity so the large position
        // jump is invisible, then restore after one engine frame.
        const mod = moduleRef.current
        if (mod?.ccall) {
          let sens = '5'
          try {
            const v = mod.ccall('Cvar_VariableString', 'string', ['string'], ['sensitivity'])
            if (v) sens = v
          } catch {}
          mod.ccall('Cbuf_AddText', null, ['string'], ['sensitivity 0\n'])
          canvas.dispatchEvent(new MouseEvent('mousemove', {
            clientX: lastX, clientY: lastY, bubbles: true,
          }))
          synthX = lastX
          synthY = lastY
          setTimeout(() => {
            try { mod.ccall('Cbuf_AddText', null, ['string'], [`sensitivity ${sens}\n`]) } catch {}
          }, 50)
        }
      }
    }
    const onMove = (e: TouchEvent) => {
      e.stopImmediatePropagation()
      e.preventDefault()
      const ct = e.targetTouches
      if (ct.length >= 2) {
        const dist = pinchLen(ct)
        const steps = Math.trunc((dist - pinchDist) / PINCH_STEP)
        if (steps !== 0) {
          for (let i = 0; i < Math.abs(steps); i++)
            canvas.dispatchEvent(new WheelEvent('wheel', {
              deltaY: steps > 0 ? -120 : 120, bubbles: true,
            }))
          pinchDist += steps * PINCH_STEP
        }
      } else if (ct.length === 1) {
        const t = ct[0]
        const dx = t.clientX - lastX
        const dy = t.clientY - lastY
        synthX += dx
        synthY += dy
        // Clamp to canvas bounds — SDL clamps internally, so if we don't
        // match, our position diverges and all deltas become zero.
        const b = canvas.getBoundingClientRect()
        synthX = Math.max(b.left, Math.min(b.right, synthX))
        synthY = Math.max(b.top, Math.min(b.bottom, synthY))
        canvas.dispatchEvent(new MouseEvent('mousemove', {
          clientX: synthX, clientY: synthY,
          movementX: dx, movementY: dy,
          bubbles: true,
        }))
        lastX = t.clientX
        lastY = t.clientY
      }
    }
    const onEnd = (e: TouchEvent) => {
      e.stopImmediatePropagation()
      e.preventDefault()
      if (scrubRef.current) {
        canvas.dispatchEvent(new MouseEvent('mousedown', {
          clientX: lastX, clientY: lastY, button: 0, bubbles: true,
        }))
        canvas.dispatchEvent(new MouseEvent('mouseup', {
          clientX: lastX, clientY: lastY, button: 0, bubbles: true,
        }))
        scrubRef.current = false
        setScrubActive(false)
        document.dispatchEvent(new KeyboardEvent('keyup', {
          code: 'ShiftLeft', key: 'ShiftLeft', bubbles: true,
        }))
      }
    }
    const onMouseUp = () => {
      if (scrubRef.current) {
        scrubRef.current = false
        setScrubActive(false)
        document.dispatchEvent(new KeyboardEvent('keyup', {
          code: 'ShiftLeft', key: 'ShiftLeft', bubbles: true,
        }))
      }
    }
    canvas.addEventListener('touchstart', onStart, true)
    canvas.addEventListener('touchmove', onMove, true)
    canvas.addEventListener('touchend', onEnd, true)
    canvas.addEventListener('mouseup', onMouseUp)
    return () => {
      canvas.removeEventListener('touchstart', onStart, true)
      canvas.removeEventListener('touchmove', onMove, true)
      canvas.removeEventListener('touchend', onEnd, true)
      canvas.removeEventListener('mouseup', onMouseUp)
    }
  }, [])

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
        // Defer vid_restart so browser paints the overlay first
        requestAnimationFrame(() => {
          mod.ccall('Cbuf_AddText', null, ['string'],
            [`r_mode -1\nr_customwidth ${w}\nr_customheight ${h}\nvid_restart\n`])
          // Schedule after Cbuf_AddText so the callback fires on the
          // engine frame AFTER vid_restart executes (our rAF runs after
          // the engine's rAF for this browser frame, so the next
          // postMainLoop is the one following the restart).
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

  // Close player list when tapping outside on mobile
  useEffect(() => {
    playerListOpenRef.current = playerListOpen
  }, [playerListOpen])
  useEffect(() => {
    if (!playerListOpen) return
    const handler = (e: TouchEvent) => {
      if (playerWrapRef.current && !playerWrapRef.current.contains(e.target as Node)) {
        setPlayerListOpen(false)
      }
    }
    document.addEventListener('touchstart', handler)
    return () => document.removeEventListener('touchstart', handler)
  }, [playerListOpen])

  // SDL2/Emscripten registers keyboard events on document, not canvas
  const sendKey = useCallback((code: string, type: 'keydown' | 'keyup') => {
    document.dispatchEvent(new KeyboardEvent(type, { code, key: code, bubbles: true }))
  }, [])

  const sendMouse = useCallback((button: number, type: 'mousedown' | 'mouseup') => {
    canvasRef.current?.dispatchEvent(new MouseEvent(type, { button, bubbles: true }))
  }, [])

  const preventFocus = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
  }, [])

  const refreshPlayerList = useCallback(() => {
    const mod = moduleRef.current
    if (!mod?.ccall) return
    try {
      const raw: string = mod.ccall('CL_TV_GetPlayerList', 'string', [], [])
      if (!raw) return
      const lines = raw.split('\n').filter(Boolean)
      if (lines.length < 1) return
      setViewpoint(parseInt(lines[0], 10))
      const players = lines.slice(1).map(line => {
        const [num, name, team, model, vr] = line.split('\t')
        return { clientNum: parseInt(num, 10), name: name || '', team: parseInt(team, 10), model: model || '', isVR: vr === '1' }
      })
      players.sort((a, b) => a.team - b.team || a.clientNum - b.clientNum)
      setPlayerList(players)
    } catch {}
  }, [])

  const handleScrubToggle = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    setScrubActive(prev => {
      const next = !prev
      scrubRef.current = next
      sendKey('ShiftLeft', next ? 'keydown' : 'keyup')
      return next
    })
  }, [sendKey])

  // Hold button helpers — includes preventDefault to keep focus on canvas
  const holdHandlers = useCallback((downFn: () => void, upFn: () => void) => ({
    onMouseDown: (e: React.MouseEvent) => { e.preventDefault(); downFn() },
    onMouseUp: () => upFn(),
    onMouseLeave: () => upFn(),
    onTouchStart: (e: React.TouchEvent) => { e.preventDefault(); downFn() },
    onTouchEnd: () => upFn(),
  }), [])

  if (error) {
    return (
      <div className="demo-player-page">
        <div className="demo-player-error">
          <p>{error}</p>
          <Link to={`/matches/${id}`}>Back to match</Link>
        </div>
      </div>
    )
  }

  return (
    <div className="demo-player-page">
      <Link to={`/matches/${id}`} className="demo-back-link">
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
      {mapName && !engineReady && (
        <div
          className="demo-levelshot"
          style={{ backgroundImage: `url(/assets/levelshots/${mapName.toLowerCase()}.jpg)` }}
        />
      )}

      <div className="demo-controls-bar" onContextMenu={e => e.preventDefault()}>
        {/* Transport */}
        <div className="ctrl-group">
          <button className="ctrl-btn" title="Rewind"
            {...holdHandlers(
              () => sendKey('ArrowLeft', 'keydown'),
              () => sendKey('ArrowLeft', 'keyup')
            )}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M11 18V6l-7 6 7 6zm7 0V6l-7 6 7 6z"/></svg>
          </button>
          <button className="ctrl-btn" title="Pause" onMouseDown={preventFocus}
            onClick={() => { sendKey('ArrowDown', 'keydown'); setTimeout(() => sendKey('ArrowDown', 'keyup'), 50) }}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z"/></svg>
          </button>
          <button className="ctrl-btn" title="Forward"
            {...holdHandlers(
              () => sendKey('ArrowRight', 'keydown'),
              () => sendKey('ArrowRight', 'keyup')
            )}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M6 18V6l7 6-7 6zm7 0V6l7 6-7 6z"/></svg>
          </button>
        </div>

        {/* View */}
        <div className="ctrl-group">
          <div ref={playerWrapRef} className={`ctrl-player-wrap${playerListOpen ? ' open' : ''}`} onMouseEnter={refreshPlayerList}>
            <button className="ctrl-btn" title="Follow player" onMouseDown={preventFocus}
              onTouchStart={e => { e.preventDefault(); refreshPlayerList(); setPlayerListOpen(prev => !prev) }}
            >
              <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z"/></svg>
            </button>
            <div className="ctrl-player-list">
              {playerList.map(p => (
                <button
                  key={p.clientNum}
                  className={`ctrl-player-item${p.clientNum === viewpoint ? ' active' : ''}${p.team === 1 || p.team === 2 ? ` team-${p.team}` : ''}`}
                  onMouseDown={e => {
                    e.preventDefault()
                    e.stopPropagation()
                    const mod = moduleRef.current
                    if (mod?.ccall) {
                      mod.ccall('Cbuf_AddText', null, ['string'], [`tv_view ${p.clientNum}\n`])
                      setViewpoint(p.clientNum)
                    }
                  }}
                  onMouseUp={e => e.stopPropagation()}
                  onClick={e => e.stopPropagation()}
                  onTouchStart={e => {
                    e.preventDefault()
                    e.stopPropagation()
                    const mod = moduleRef.current
                    if (mod?.ccall) {
                      mod.ccall('Cbuf_AddText', null, ['string'], [`tv_view ${p.clientNum}\n`])
                      setViewpoint(p.clientNum)
                    }
                  }}
                >
                  <span className="ctrl-player-vr-slot">
                    {p.isVR && <img src="/assets/vr/vr.png" alt="VR" />}
                  </span>
                  <PlayerPortrait model={p.model} size="sm" />
                  <ColoredText text={p.name} />
                </button>
              ))}
            </div>
          </div>
          <button className="ctrl-btn" title="Toggle camera" onMouseDown={preventFocus}
            onClick={() => { sendMouse(2, 'mousedown'); setTimeout(() => sendMouse(2, 'mouseup'), 50) }}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z"/></svg>
          </button>
          <button className="ctrl-btn" title="Recenter view" onMouseDown={preventFocus}
            onClick={() => { sendKey('ArrowUp', 'keydown'); setTimeout(() => sendKey('ArrowUp', 'keyup'), 50) }}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="3"/><line x1="12" y1="2" x2="12" y2="6"/><line x1="12" y1="18" x2="12" y2="22"/><line x1="2" y1="12" x2="6" y2="12"/><line x1="18" y1="12" x2="22" y2="12"/></svg>
          </button>
        </div>

        {/* Toggle / Hold */}
        <div className="ctrl-group">
          <button className={`ctrl-btn${scrubActive ? ' active' : ''}`} title="Scrub mode"
            onMouseDown={handleScrubToggle}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2"><line x1="3" y1="12" x2="21" y2="12"/><circle cx="12" cy="12" r="3" fill="currentColor"/></svg>
          </button>
          <button className="ctrl-btn" title="Show scoreboard"
            {...holdHandlers(
              () => sendKey('Tab', 'keydown'),
              () => sendKey('Tab', 'keyup')
            )}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="3" y1="15" x2="21" y2="15"/><line x1="12" y1="3" x2="12" y2="21"/></svg>
          </button>
        </div>
        <div className="ctrl-group ctrl-help-wrap">
          <button className="ctrl-btn ctrl-help-btn" onMouseDown={preventFocus}>?</button>
          <div className="ctrl-help-tooltip">
            <div><kbd>←</kbd> / <kbd>→</kbd> Rewind / Forward</div>
            <div><kbd>↓</kbd> Pause</div>
            <div><kbd>↑</kbd> Recenter view</div>
            <div><kbd>Click</kbd> Follow next player</div>
            <div><kbd>Right-click</kbd> Toggle camera</div>
            <div><kbd>Scroll</kbd> Zoom in/out</div>
            <div><kbd>Shift + Click</kbd> Scrub timeline</div>
            <div><kbd>Tab</kbd> Scoreboard</div>
            <div><kbd>W/A/S/D</kbd> Move camera</div>
            <div><kbd>Space</kbd> / <kbd>C</kbd> Up / Down</div>
            <div><kbd>Mouse</kbd> Look around</div>
          </div>
        </div>
      </div>

      <div className="demo-move-pad" onContextMenu={e => e.preventDefault()}>
        <button className="move-btn" {...holdHandlers(() => sendKey('Space', 'keydown'), () => sendKey('Space', 'keyup'))}>↑</button>
        <button className="move-btn" {...holdHandlers(() => sendKey('KeyW', 'keydown'), () => sendKey('KeyW', 'keyup'))}>W</button>
        <button className="move-btn" {...holdHandlers(() => sendKey('KeyC', 'keydown'), () => sendKey('KeyC', 'keyup'))}>↓</button>
        <button className="move-btn" {...holdHandlers(() => sendKey('KeyA', 'keydown'), () => sendKey('KeyA', 'keyup'))}>A</button>
        <button className="move-btn" {...holdHandlers(() => sendKey('KeyS', 'keydown'), () => sendKey('KeyS', 'keyup'))}>S</button>
        <button className="move-btn" {...holdHandlers(() => sendKey('KeyD', 'keydown'), () => sendKey('KeyD', 'keyup'))}>D</button>
      </div>
    </div>
  )
}
