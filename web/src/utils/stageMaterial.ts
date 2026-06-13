export interface StageManifest {
  maps: string[];
  animFreq?: number;
  blend: "opaque" | "add" | "blend" | "filter";
  alphaFunc?: "ge128" | "gt0" | "lt128" | "";
  tcGen?: "environment" | "";
  tcMod?: { type: string; args: number[] }[];
  rgbGen: "identity" | "lightingDiffuse";
  clamp?: boolean;
  doubleSided?: boolean;
  deform?: string;
}

export interface StageDescriptor {
  blending: "normal" | "additive" | "multiply";
  transparent: boolean;
  depthWrite: boolean;
  alphaTest: number;
  doubleSided: boolean;
  lit: boolean;
  envMap: boolean;
  clamp: boolean;
  animFreq: number;
  scroll: [number, number] | null;
  scale: [number, number] | null;
  rotate: number | null;
}

export function stageDescriptor(s: StageManifest): StageDescriptor {
  const blending =
    s.blend === "add"
      ? "additive"
      : s.blend === "filter"
        ? "multiply"
        : "normal";
  const transparent =
    s.blend === "add" || s.blend === "blend" || s.blend === "filter";
  const alphaTest =
    s.alphaFunc === "ge128" ? 0.5 : s.alphaFunc === "gt0" ? 0.0001 : 0;
  const find = (t: string) => s.tcMod?.find((m) => m.type === t)?.args ?? null;
  const scrollArgs = find("scroll");
  const scaleArgs = find("scale");
  const rotateArgs = find("rotate");
  return {
    blending,
    transparent,
    depthWrite: !transparent,
    alphaTest,
    doubleSided: !!s.doubleSided,
    lit: s.rgbGen === "lightingDiffuse",
    envMap: s.tcGen === "environment",
    clamp: !!s.clamp,
    animFreq: s.animFreq ?? 0,
    scroll: scrollArgs ? [scrollArgs[0], scrollArgs[1]] : null,
    scale: scaleArgs ? [scaleArgs[0], scaleArgs[1]] : null,
    rotate: rotateArgs ? rotateArgs[0] : null,
  };
}
