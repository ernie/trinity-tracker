// Browsers allow audio only after a user gesture, and count only some events
// as one (a click or a lifted touch, not a touch's pointerdown).
let ctx: AudioContext | null = null;
const files = new Map<string, Promise<ArrayBuffer | null>>();
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
  for (const type of ["pointerup", "click", "touchend", "keydown"]) {
    window.addEventListener(type, unlock, true);
  }
}

export function preloadHonk(url: string): void {
  if (!files.has(url)) {
    files.set(
      url,
      fetch(url)
        .then((r) => (r.ok ? r.arrayBuffer() : null))
        .catch(() => null),
    );
  }
}

function decoded(
  audio: AudioContext,
  url: string,
): Promise<AudioBuffer | null> {
  let p = buffers.get(url);
  if (!p) {
    preloadHonk(url);
    p = files
      .get(url)!
      .then((b) => (b ? audio.decodeAudioData(b.slice(0)) : null))
      .catch(() => null);
    buffers.set(url, p);
  }
  return p;
}

// A honk started by the gesture that unlocks audio plays once the context
// finishes resuming.
export function playHonk(url: string): void {
  const audio = ctx;
  if (!soundOn || !audio || audio.state === "closed") return;
  void decoded(audio, url).then((buffer) => {
    if (!buffer) return;
    const src = audio.createBufferSource();
    src.buffer = buffer;
    src.connect(audio.destination);
    src.start();
  });
}
