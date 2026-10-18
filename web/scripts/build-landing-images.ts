// Generates WebP and AVIF sizes of the landing-page wallpaper. Run it
// again after the source PNG changes; the outputs are gitignored.
//
// Usage: bun scripts/build-landing-images.ts (run from web/)
import sharp from "sharp";
import { mkdirSync } from "node:fs";
import { resolve } from "node:path";

const ROOT = resolve(import.meta.dir, "..");
const SRC = resolve(ROOT, "public/assets/landing/wallpaper-wqhd.png");
const OUT_DIR = resolve(ROOT, "public/assets/landing");

mkdirSync(OUT_DIR, { recursive: true });

interface Variant {
  width: number;
  filename: string;
  format: "webp" | "avif";
  quality: number;
  cropToMobile?: boolean;
}

const variants: Variant[] = [
  { width: 2880, filename: "wallpaper-2880.webp", format: "webp", quality: 78 },
  { width: 1920, filename: "wallpaper-1920.webp", format: "webp", quality: 78 },
  {
    width: 1200,
    filename: "wallpaper-1200.webp",
    format: "webp",
    quality: 78,
    cropToMobile: true,
  },
  { width: 2880, filename: "wallpaper-2880.avif", format: "avif", quality: 60 },
  { width: 1920, filename: "wallpaper-1920.avif", format: "avif", quality: 60 },
  {
    width: 1200,
    filename: "wallpaper-1200.avif",
    format: "avif",
    quality: 60,
    cropToMobile: true,
  },
];

for (const v of variants) {
  let img = sharp(SRC).resize(v.width);
  if (v.cropToMobile) {
    // The 3440×1440 source crops to 16:9 around the logo, which sits
    // about 38% of the way down.
    const aspectH = Math.round(v.width / (16 / 9));
    img = sharp(SRC).resize({
      width: v.width,
      height: aspectH,
      fit: "cover",
      position: "center",
    });
  }
  const opts = { quality: v.quality };
  const buf =
    v.format === "webp"
      ? await img.webp(opts).toBuffer()
      : await img.avif(opts).toBuffer();
  await Bun.write(resolve(OUT_DIR, v.filename), buf);
  console.log(`wrote ${v.filename} (${(buf.length / 1024).toFixed(0)}KB)`);
}
