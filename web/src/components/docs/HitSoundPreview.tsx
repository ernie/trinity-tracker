import { useRef, useState } from 'react'
import { PlayIcon } from '../PlayIcon'

// Compact preview widget for a single hit sound — used in the
// /docs/play Hit Sounds entry to let readers audition each
// damage-tier tone without the full native <audio controls>
// surface (which is wider, includes a progress bar that's
// pointless for sub-second clips, and starts at 100% volume).
//
// Defaults to 50% volume because the source wavs are short
// and punchy enough that 100% is uncomfortable on headphones.
// Slider is uncontrolled (defaultValue) so React doesn't
// re-render mid-drag; volume state is tracked separately so
// onPlay can read the latest value.
export function HitSoundPreview({ src }: { src: string }) {
  const audioRef = useRef<HTMLAudioElement>(null)
  const [volume, setVolume] = useState(0.5)

  const onPlay = () => {
    const a = audioRef.current
    if (!a) return
    a.currentTime = 0
    a.volume = volume
    a.play()
  }

  const onVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const v = Number(e.target.value) / 100
    setVolume(v)
    if (audioRef.current) audioRef.current.volume = v
  }

  return (
    <div className="hitsound-preview">
      <button
        type="button"
        onClick={onPlay}
        className="hitsound-preview__play"
        aria-label="Play hit sound"
      >
        <PlayIcon size={10} />
      </button>
      <input
        type="range"
        min="0"
        max="100"
        defaultValue="50"
        onChange={onVolumeChange}
        className="hitsound-preview__volume"
        aria-label="Volume"
        title="Volume"
      />
      <audio ref={audioRef} src={src} preload="none" />
    </div>
  )
}
