import * as THREE from "three";

// Faithful port of the engine's deformVertexes autosprite/autosprite2
// (tr_shade_calc.c). A deform surface is quads of 4 verts; each frame every
// quad is re-oriented to the view. autosprite rebuilds a camera-facing quad
// (fresh 0-1 UVs); autosprite2 pivots a rectangle around its long axis (the
// line through the midpoints of its two shortest edges), keeping that axis
// fixed and the original UVs. View axes are uniform (not per-vertex).

const EDGE_VERTS: [number, number][] = [
  [0, 1],
  [0, 2],
  [0, 3],
  [1, 2],
  [1, 3],
  [2, 3],
];

export interface Billboard {
  geometry: THREE.BufferGeometry;
  update(forward: THREE.Vector3, left: THREE.Vector3, up: THREE.Vector3): void;
}

interface SpriteQuad {
  out: number;
  mid: THREE.Vector3;
  radius: number;
}

interface Sprite2Edge {
  va: number;
  vb: number;
  mid: THREE.Vector3;
  half: number;
  sign: number;
}
interface Sprite2Quad {
  major: THREE.Vector3;
  edges: [Sprite2Edge, Sprite2Edge];
}

export function buildBillboard(
  positions: Float32Array,
  uvs: Float32Array,
  indices: number[],
  deform: string,
): Billboard {
  const quadCount = Math.floor(positions.length / 3 / 4);
  const outPos = new Float32Array(quadCount * 4 * 3);
  const outUv = new Float32Array(quadCount * 4 * 2);
  const outIdx: number[] = [];
  const vec = (i: number) =>
    new THREE.Vector3(
      positions[i * 3],
      positions[i * 3 + 1],
      positions[i * 3 + 2],
    );

  const geometry = new THREE.BufferGeometry();
  const posAttr = new THREE.BufferAttribute(outPos, 3);
  geometry.setAttribute("position", posAttr);
  geometry.setAttribute("uv", new THREE.BufferAttribute(outUv, 2));
  geometry.setAttribute(
    "normal",
    new THREE.BufferAttribute(new Float32Array(quadCount * 4 * 3), 3),
  );

  const isAxis = deform === "autosprite2";
  const sprites: SpriteQuad[] = [];
  const sprite2s: Sprite2Quad[] = [];

  for (let q = 0; q < quadCount; q++) {
    const base = q * 4;
    const out = q * 4;
    const p = [vec(base), vec(base + 1), vec(base + 2), vec(base + 3)];

    if (!isAxis) {
      const mid = new THREE.Vector3()
        .add(p[0])
        .add(p[1])
        .add(p[2])
        .add(p[3])
        .multiplyScalar(0.25);
      const radius = p[0].distanceTo(mid) * 0.707;
      sprites.push({ out, mid, radius });
      outUv.set([0, 0, 1, 0, 1, 1, 0, 1], out * 2);
      outIdx.push(out, out + 1, out + 2, out, out + 2, out + 3);
      continue;
    }

    const lensq = EDGE_VERTS.map(([a, b]) => p[a].distanceToSquared(p[b]));
    let n0 = 0;
    let n1 = 0;
    let l0 = Infinity;
    let l1 = Infinity;
    for (let j = 0; j < 6; j++) {
      if (lensq[j] < l0) {
        n1 = n0;
        l1 = l0;
        n0 = j;
        l0 = lensq[j];
      } else if (lensq[j] < l1) {
        n1 = j;
        l1 = lensq[j];
      }
    }
    const nums = [n0, n1];
    const lens = [l0, l1];
    const mids = nums.map((n) =>
      p[EDGE_VERTS[n][0]].clone().add(p[EDGE_VERTS[n][1]]).multiplyScalar(0.5),
    );
    const major = mids[1].clone().sub(mids[0]);

    for (let k = 0; k < 4; k++) {
      outUv[(out + k) * 2] = uvs[(base + k) * 2];
      outUv[(out + k) * 2 + 1] = uvs[(base + k) * 2 + 1];
    }
    const qi = indices.slice(q * 6, q * 6 + 6);
    for (const li of qi) outIdx.push(out + li);

    const edges = nums.map((n, j): Sprite2Edge => {
      const a = EDGE_VERTS[n][0];
      const b = EDGE_VERTS[n][1];
      let forwardUse = false;
      for (let k = 0; k < 5; k++) {
        if (qi[k] === a && qi[k + 1] === b) {
          forwardUse = true;
          break;
        }
      }
      const half = 0.5 * Math.sqrt(lens[j]);
      return {
        va: out + a,
        vb: out + b,
        mid: mids[j],
        half,
        sign: forwardUse ? -1 : 1,
      };
    }) as [Sprite2Edge, Sprite2Edge];
    sprite2s.push({ major, edges });
  }

  geometry.setIndex(outIdx);

  const minor = new THREE.Vector3();
  const tmp = new THREE.Vector3();
  const setPos = (vi: number, v: THREE.Vector3) => {
    outPos[vi * 3] = v.x;
    outPos[vi * 3 + 1] = v.y;
    outPos[vi * 3 + 2] = v.z;
  };

  return {
    geometry,
    update(forward, left, up) {
      if (isAxis) {
        for (const s of sprite2s) {
          minor.crossVectors(s.major, forward).normalize();
          for (const e of s.edges) {
            setPos(
              e.va,
              tmp.copy(e.mid).addScaledVector(minor, e.sign * e.half),
            );
            setPos(
              e.vb,
              tmp.copy(e.mid).addScaledVector(minor, -e.sign * e.half),
            );
          }
        }
      } else {
        for (const s of sprites) {
          const lx = left.x * s.radius;
          const ly = left.y * s.radius;
          const lz = left.z * s.radius;
          const ux = up.x * s.radius;
          const uy = up.y * s.radius;
          const uz = up.z * s.radius;
          const m = s.mid;
          outPos.set(
            [
              m.x + lx + ux,
              m.y + ly + uy,
              m.z + lz + uz,
              m.x - lx + ux,
              m.y - ly + uy,
              m.z - lz + uz,
              m.x - lx - ux,
              m.y - ly - uy,
              m.z - lz - uz,
              m.x + lx - ux,
              m.y + ly - uy,
              m.z + lz - uz,
            ],
            s.out * 3,
          );
        }
      }
      posAttr.needsUpdate = true;
    },
  };
}
