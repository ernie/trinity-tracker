import { expect, test } from "bun:test";
import { stageDescriptor } from "./stageMaterial";

test("opaque lit stage writes depth, no blend, lit", () => {
  const d = stageDescriptor({
    maps: ["band.png"],
    blend: "opaque",
    rgbGen: "lightingDiffuse",
  });
  expect(d.blending).toBe("normal");
  expect(d.transparent).toBe(false);
  expect(d.depthWrite).toBe(true);
  expect(d.lit).toBe(true);
  expect(d.alphaTest).toBe(0);
});

test("alphaFunc ge128 sets alphaTest 0.5", () => {
  const d = stageDescriptor({
    maps: ["f.png"],
    blend: "opaque",
    alphaFunc: "ge128",
    rgbGen: "lightingDiffuse",
    doubleSided: true,
  });
  expect(d.alphaTest).toBeCloseTo(0.5, 5);
  expect(d.doubleSided).toBe(true);
});

test("additive env stage: additive blend, no depth write, reflective", () => {
  const d = stageDescriptor({
    maps: ["tinfx2b.png"],
    blend: "add",
    tcGen: "environment",
    rgbGen: "lightingDiffuse",
  });
  expect(d.blending).toBe("additive");
  expect(d.depthWrite).toBe(false);
  expect(d.transparent).toBe(true);
  expect(d.envMap).toBe(true);
});

test("scroll tcMod surfaces as a per-second offset rate", () => {
  const d = stageDescriptor({
    maps: ["snow.png"],
    blend: "opaque",
    rgbGen: "identity",
    tcMod: [
      { type: "scale", args: [0.5, 0.5] },
      { type: "scroll", args: [9, 0.3] },
    ],
  });
  expect(d.scroll).toEqual([9, 0.3]);
  expect(d.scale).toEqual([0.5, 0.5]);
  expect(d.lit).toBe(false);
});
