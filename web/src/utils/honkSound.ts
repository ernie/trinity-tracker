// Browsers allow audio only after a click, tap or key press, so the context
// waits for the first one.
let ctx: AudioContext | null = null;
const buffers = new Map<string, Promise<AudioBuffer | null>>();

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
  if (!audio || audio.state !== "running") return;
  void load(url).then((buffer) => {
    if (!buffer) return;
    const src = audio.createBufferSource();
    src.buffer = buffer;
    src.connect(audio.destination);
    src.start();
  });
}
