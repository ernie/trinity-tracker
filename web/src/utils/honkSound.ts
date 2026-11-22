// Browsers allow audio only after a click, tap or key press, so the context
// waits for the first one.
let ctx: AudioContext | null = null;
const buffers = new Map<string, Promise<AudioBuffer | null>>();

const STORAGE_KEY = "honk_sound";
const listeners = new Set<() => void>();
let soundOn = true;
try {
  soundOn = localStorage.getItem(STORAGE_KEY) !== "off";
} catch {
  // Storage can be blocked; the sound stays on for this visit.
}

export function isHonkSoundOn(): boolean {
  return soundOn;
}

export function setHonkSoundOn(on: boolean): void {
  soundOn = on;
  try {
    localStorage.setItem(STORAGE_KEY, on ? "on" : "off");
  } catch {
    // Storage can be blocked; the choice holds for this visit.
  }
  listeners.forEach((l) => l());
}

export function subscribeHonkSound(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function unlock(): void {
  if (!ctx) ctx = new AudioContext();
  if (ctx.state !== "running") void ctx.resume();
}

if (typeof window !== "undefined") {
  window.addEventListener("pointerdown", unlock, true);
  window.addEventListener("keydown", unlock, true);
}

function load(url: string): Promise<AudioBuffer | null> {
  let p = buffers.get(url);
  if (!p) {
    p = fetch(url)
      .then((r) => (r.ok ? r.arrayBuffer() : Promise.reject(r.status)))
      .then((b) => ctx!.decodeAudioData(b))
      .catch(() => null);
    buffers.set(url, p);
  }
  return p;
}

export function playHonk(url: string): void {
  const audio = ctx;
  if (!soundOn || !audio || audio.state !== "running") return;
  void load(url).then((buffer) => {
    if (!buffer) return;
    const src = audio.createBufferSource();
    src.buffer = buffer;
    src.connect(audio.destination);
    src.start();
  });
}
