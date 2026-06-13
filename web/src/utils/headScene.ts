import * as THREE from "three";
import { stageDescriptor, type StageManifest } from "./stageMaterial";
import { buildBillboard, type Billboard } from "./billboard";
import { HEAD_OVERFLOW } from "./headRenderMath";

interface Framing {
  cx: number;
  cy: number;
  cz: number;
  dist: number;
}

// Per-model framing overrides (hand-tuned via the head tuner), fetched once.
let framingOverrides: Promise<Record<string, Partial<Framing>>> | null = null;
function loadFramingOverrides(): Promise<Record<string, Partial<Framing>>> {
  if (!framingOverrides) {
    framingOverrides = fetch("/assets/heads/framing.json")
      .then((r) => (r.ok ? r.json() : {}))
      .catch(() => ({}));
  }
  return framingOverrides;
}

interface HeadManifest {
  bounds: { mins: [number, number, number]; maxs: [number, number, number] };
  vertexCount: number;
  indexCount: number;
  surfaces: {
    name: string;
    vertOffset: number;
    vertCount: number;
    idxOffset: number;
    idxCount: number;
  }[];
  skins: Record<string, Record<string, StageManifest[]>>;
  headOffset?: [number, number, number];
}

interface StageRT {
  material: THREE.Material;
  textures: THREE.Texture[];
  animFreq: number;
  scroll: [number, number] | null;
  setFrame: (t: THREE.Texture) => void;
}

export class HeadScene {
  readonly scene = new THREE.Scene();
  readonly camera: THREE.PerspectiveCamera;
  private headGroup = new THREE.Group();
  private subGeometries: THREE.BufferGeometry[] = [];
  private billboards: Billboard[] = [];
  private envUniforms: { value: THREE.Vector3 }[] = [];
  private fwd = new THREE.Vector3();
  private lft = new THREE.Vector3();
  private upv = new THREE.Vector3();
  private framing: Framing = { cx: 0, cy: 0, cz: 0, dist: 20 };
  private stages: StageRT[] = [];

  constructor() {
    this.camera = new THREE.PerspectiveCamera(30, 1, 1, 4096);
    // Quake model space is Z-up, X-forward; keep meshes in that space and
    // orient the camera to match rather than rotating the geometry.
    this.camera.up.set(0, 0, 1);
    this.scene.add(this.headGroup);
    this.scene.add(new THREE.AmbientLight(0xffffff, 1.1));
    const key = new THREE.DirectionalLight(0xffffff, 0.8);
    key.position.set(1, 0.6, 0.8);
    this.scene.add(key);
    const fill = new THREE.DirectionalLight(0xffffff, 0.4);
    fill.position.set(-0.6, 0.3, 0.5);
    this.scene.add(fill);
  }

  async load(modelName: string, skin: string): Promise<void> {
    const base = `/assets/heads/${modelName}`;
    const manifest = (await fetch(`${base}/head.json`).then((r) => {
      if (!r.ok) throw new Error(`head.json ${r.status}`);
      return r.json();
    })) as HeadManifest;
    const buf = await fetch(`${base}/head.geo`).then((r) => {
      if (!r.ok) throw new Error(`head.geo ${r.status}`);
      return r.arrayBuffer();
    });

    const vBytes = manifest.vertexCount * 8 * 4;
    const verts = new Float32Array(buf, 0, manifest.vertexCount * 8);
    const srcIndices = new Uint16Array(buf, vBytes, manifest.indexCount);
    // Quake winds triangles opposite to three's front-face convention; flip
    // each so FrontSide shows the outward faces.
    const indices = new Uint16Array(srcIndices.length);
    for (let i = 0; i + 2 < srcIndices.length; i += 3) {
      indices[i] = srcIndices[i];
      indices[i + 1] = srcIndices[i + 2];
      indices[i + 2] = srcIndices[i + 1];
    }
    const interleaved = new THREE.InterleavedBuffer(verts, 8);
    const indexAttr = new THREE.BufferAttribute(indices, 1);

    const buildSubGeometry = (
      idxOffset: number,
      idxCount: number,
    ): THREE.BufferGeometry => {
      const g = new THREE.BufferGeometry();
      g.setAttribute(
        "position",
        new THREE.InterleavedBufferAttribute(interleaved, 3, 0),
      );
      g.setAttribute(
        "normal",
        new THREE.InterleavedBufferAttribute(interleaved, 3, 3),
      );
      g.setAttribute(
        "uv",
        new THREE.InterleavedBufferAttribute(interleaved, 2, 6),
      );
      g.setIndex(indexAttr);
      g.setDrawRange(idxOffset, idxCount);
      return g;
    };

    const skinMap =
      manifest.skins[skin] ??
      manifest.skins[Object.keys(manifest.skins)[0]] ??
      {};

    // Framing ported from CG_DrawHead: centre by the head bounds and pull the
    // camera back len/0.268 (~30° FOV), plus the model's animation.cfg
    // headoffset — the artist's per-model HUD-icon correction (forward = zoom,
    // up = vertical). Deform sprites (beams/lasers) are excluded from bounds.
    const lo = [Infinity, Infinity, Infinity];
    const hi = [-Infinity, -Infinity, -Infinity];
    let solid = false;
    for (const surf of manifest.surfaces) {
      const st = skinMap[surf.name];
      if (!st || st.length === 0 || st[0].deform) continue;
      for (let k = 0; k < surf.vertCount; k++) {
        const o = (surf.vertOffset + k) * 8;
        for (let a = 0; a < 3; a++) {
          lo[a] = Math.min(lo[a], verts[o + a]);
          hi[a] = Math.max(hi[a], verts[o + a]);
        }
      }
      solid = true;
    }
    if (!solid) {
      lo.splice(0, 3, ...manifest.bounds.mins);
      hi.splice(0, 3, ...manifest.bounds.maxs);
    }
    const ho = manifest.headOffset ?? [0, 0, 0];
    const len = 0.7 * (hi[2] - lo[2]);
    this.framing = {
      cx: 0.5 * (lo[0] + hi[0]),
      cy: 0.5 * (lo[1] + hi[1]),
      cz: 0.5 * (lo[2] + hi[2]) - ho[2],
      dist: len / 0.268 + ho[0],
    };
    const overrides = await loadFramingOverrides();
    if (overrides[modelName]) Object.assign(this.framing, overrides[modelName]);
    this.applyFraming();
    const loader = new THREE.TextureLoader();
    const loadTex = (png: string, clamp: boolean) =>
      new Promise<THREE.Texture>((resolve, reject) => {
        if (png === "$white") {
          const white = new THREE.DataTexture(
            new Uint8Array([255, 255, 255, 255]),
            1,
            1,
          );
          white.needsUpdate = true;
          resolve(white);
          return;
        }
        loader.load(
          `${base}/${png}`,
          (t) => {
            t.colorSpace = THREE.SRGBColorSpace;
            // MD3 texcoords use a top-left origin; three flips Y by default.
            t.flipY = false;
            t.wrapS = t.wrapT = clamp
              ? THREE.ClampToEdgeWrapping
              : THREE.RepeatWrapping;
            resolve(t);
          },
          undefined,
          reject,
        );
      });

    let order = 0;
    for (const surf of manifest.surfaces) {
      const stages = skinMap[surf.name];
      if (!stages || stages.length === 0) continue;
      const deform = stages[0].deform;
      let subGeo: THREE.BufferGeometry;
      if (deform === "autosprite" || deform === "autosprite2") {
        const n = surf.vertCount;
        const pos = new Float32Array(n * 3);
        const uv = new Float32Array(n * 2);
        for (let k = 0; k < n; k++) {
          const src = (surf.vertOffset + k) * 8;
          pos[k * 3] = verts[src];
          pos[k * 3 + 1] = verts[src + 1];
          pos[k * 3 + 2] = verts[src + 2];
          uv[k * 2] = verts[src + 6];
          uv[k * 2 + 1] = verts[src + 7];
        }
        const localIdx: number[] = [];
        for (let k = 0; k < surf.idxCount; k++) {
          localIdx.push(srcIndices[surf.idxOffset + k] - surf.vertOffset);
        }
        const bb = buildBillboard(pos, uv, localIdx, deform);
        this.billboards.push(bb);
        subGeo = bb.geometry;
      } else {
        subGeo = buildSubGeometry(surf.idxOffset, surf.idxCount);
      }
      this.subGeometries.push(subGeo);
      for (const stage of stages) {
        const d = stageDescriptor(stage);
        const textures = await Promise.all(
          stage.maps.map((m) => loadTex(m, d.clamp)),
        );
        if (d.scale)
          textures.forEach((t) => t.repeat.set(d.scale![0], d.scale![1]));

        // tcGen environment reflects the texture by the view-space reflection
        // vector (ported from RB_CalcEnvironmentTexCoords). Lit stages light
        // the diffuse; identity stages draw the texture unlit.
        let material: THREE.Material;
        let setFrame: (t: THREE.Texture) => void;
        if (d.envMap) {
          textures.forEach((t) => {
            t.wrapS = t.wrapT = THREE.ClampToEdgeWrapping;
          });
          const m = new THREE.MeshBasicMaterial({ map: textures[0] });
          material = m;
          setFrame = (t) => {
            m.map = t;
            m.needsUpdate = true;
          };
        } else if (d.lit) {
          const m = new THREE.MeshStandardMaterial({
            map: textures[0],
            roughness: 1,
            metalness: 0,
          });
          material = m;
          setFrame = (t) => {
            m.map = t;
            m.needsUpdate = true;
          };
        } else {
          const m = new THREE.MeshBasicMaterial({ map: textures[0] });
          material = m;
          setFrame = (t) => {
            m.map = t;
            m.needsUpdate = true;
          };
        }
        material.transparent = d.transparent;
        material.depthWrite = d.depthWrite;
        material.alphaTest = d.alphaTest;
        material.side = d.doubleSided ? THREE.DoubleSide : THREE.FrontSide;
        if (d.blending === "additive") {
          // Q3 add is GL_ONE GL_ONE. The beam texture has no usable alpha, so
          // canvas alpha comes from luminance (see onBeforeCompile): black → 0
          // (transparent over the page), bright → visible even off the head.
          material.blending = THREE.CustomBlending;
          material.blendEquation = THREE.AddEquation;
          material.blendSrc = THREE.OneFactor;
          material.blendDst = THREE.OneFactor;
          material.blendSrcAlpha = THREE.OneFactor;
          material.blendDstAlpha = THREE.OneFactor;
        } else if (d.blending === "multiply") {
          // Multiply RGB into the destination but keep the destination's alpha,
          // else the multiply zeroes canvas alpha and Chromium drops the face.
          material.blending = THREE.CustomBlending;
          material.blendEquation = THREE.AddEquation;
          material.blendSrc = THREE.ZeroFactor;
          material.blendDst = THREE.SrcColorFactor;
          material.blendSrcAlpha = THREE.ZeroFactor;
          material.blendDstAlpha = THREE.OneFactor;
        } else {
          material.blending = THREE.NormalBlending;
        }
        const needRefl = d.envMap;
        const needLum = d.blending === "additive";
        if (needRefl || needLum) {
          material.onBeforeCompile = (shader) => {
            if (needRefl) {
              shader.uniforms.uCamObj = { value: new THREE.Vector3() };
              this.envUniforms.push(shader.uniforms.uCamObj);
              shader.vertexShader =
                "uniform vec3 uCamObj;\n" +
                shader.vertexShader.replace(
                  "#include <uv_vertex>",
                  "#include <uv_vertex>\n\t{ vec3 ev = normalize(uCamObj - position); float ed = dot(normal, ev); vec3 er = normal * 2.0 * ed - ev; vMapUv = vec2(0.5 + er.y * 0.5, 0.5 - er.z * 0.5); }",
                );
            }
            if (needLum) {
              shader.fragmentShader = shader.fragmentShader.replace(
                "#include <colorspace_fragment>",
                "#include <colorspace_fragment>\n\tgl_FragColor.a = max(gl_FragColor.r, max(gl_FragColor.g, gl_FragColor.b));",
              );
            }
          };
        }
        const mesh = new THREE.Mesh(subGeo, material);
        mesh.renderOrder = order++;
        this.headGroup.add(mesh);
        this.stages.push({
          material,
          textures,
          animFreq: d.animFreq,
          scroll: d.scroll,
          setFrame,
        });
      }
    }
  }

  // headAngles in degrees: yaw about Z (up), pitch about Y. Base yaw 180
  // faces the camera, so subtract it to keep the idle drift centred.
  update(yawDeg: number, pitchDeg: number, timeMs = 0): void {
    this.headGroup.rotation.z = THREE.MathUtils.degToRad(yawDeg - 180);
    this.headGroup.rotation.y = THREE.MathUtils.degToRad(pitchDeg);
    if (this.billboards.length > 0) {
      // Camera view axes (world) transformed into the rotating head's local
      // space — the engine's GlobalVectorToLocal. Camera sits on +X looking
      // toward the origin, up +Z.
      const inv = this.headGroup.quaternion.clone().invert();
      this.fwd.set(-1, 0, 0).applyQuaternion(inv);
      this.lft.set(0, -1, 0).applyQuaternion(inv);
      this.upv.set(0, 0, 1).applyQuaternion(inv);
      for (const bb of this.billboards) bb.update(this.fwd, this.lft, this.upv);
    }
    if (this.envUniforms.length > 0) {
      // Camera position in the rotating head's object space, for the
      // reflection-vector env mapping.
      this.headGroup.updateMatrixWorld(true);
      const camObj = this.headGroup.worldToLocal(this.camera.position.clone());
      for (const u of this.envUniforms) u.value.copy(camObj);
    }
    const t = timeMs / 1000;
    for (const s of this.stages) {
      if (s.scroll) {
        for (const tex of s.textures)
          tex.offset.set(s.scroll[0] * t, s.scroll[1] * t);
      }
      if (s.animFreq > 0 && s.textures.length > 1) {
        const frame = Math.floor(t * s.animFreq) % s.textures.length;
        s.setFrame(s.textures[frame]);
      }
    }
  }

  private applyFraming(): void {
    const { cx, cy, cz, dist } = this.framing;
    this.headGroup.position.set(-cx, -cy, -cz);
    // Pull back by the overflow factor so the solid head renders box-sized;
    // ModelHead.css scales the canvas up by the same factor so hair overflows.
    this.camera.position.set(dist * HEAD_OVERFLOW, 0, 0);
    this.camera.lookAt(0, 0, 0);
  }

  dispose(): void {
    for (const s of this.stages) {
      s.textures.forEach((t) => t.dispose());
      s.material.dispose();
    }
    this.subGeometries.forEach((g) => g.dispose());
  }
}
