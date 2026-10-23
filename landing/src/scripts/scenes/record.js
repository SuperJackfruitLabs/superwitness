// "How it works": an evidence table. At the back, the two products that send telemetry and the two
// stores it lands in; at the front, the run's record and a stamp. Boxes and low cylinders,
// flat-shaded, made in code. Scroll runs it in three beats, one per step of the section:
//   0. Products send OTLP: a batch goes from the products into the trace store, then one into the
//      log store. SuperWitness is not in that path; the record does not move.
//   1. Ask for a run: the record's rows fill in one at a time, the joined trace in the accent and
//      the unreported cost short, in amber, as the hero's document reads.
//   2. Record a verdict: the stamp comes down once and leaves its seal on the record, where the
//      seal stays when the stamp lifts: verdicts are append-only.
import { BoxGeometry, CylinderGeometry, Group, Mesh, MeshLambertMaterial } from 'three';

const mat = (c) => new MeshLambertMaterial({ color: c, flatShading: true });
const SEND = 0.9, FILL = 0.32, PRESS = 0.4, LIFT = 0.5;
const B0 = 2 * SEND + 0.1, B1 = B0 + 0.2 + 5 * FILL;

export const record = {
  duration: B1 + 0.15 + PRESS + LIFT,
  beats: [[0, B0], [B0, B1], [B1, B1 + 0.15 + PRESS + LIFT]],
  fit: { w: 4.2, h: 2.4 },
  look: [0, 0.35, 0.1],
  yaw: 14,
  el: 36,
  fov: 26,
  shadow: 2.8,
  lampAt: [0.2, 1.6, 0.9],
  build(accent) {
    const M = {
      stone: mat('#d6cdc1'), stoneTop: mat('#e4ddd3'), pipeline: mat('#e0a43a'), pod: mat('#5f80d0'),
      store: mat('#5d6673'), storeTop: mat('#4a525d'), slot: mat('#22272e'), batch: mat('#7cc3dd'),
      board: mat('#8a6a4f'), paper: mat('#f4f1ea'), row: mat('#c9c2b6'), warn: mat('#e2a03a'),
      accent: mat(accent), handle: mat('#4a3a2e'),
    };
    const root = new Group();
    function box(w, h, d, m, x = 0, y = 0, z = 0, parent = root) {
      const b = new Mesh(new BoxGeometry(w, h, d), m);
      b.position.set(x, y, z);
      b.castShadow = b.receiveShadow = true;
      parent.add(b);
      return b;
    }
    function cyl(r, h, m, x = 0, y = 0, z = 0, parent = root, seg = 12) {
      const c = new Mesh(new CylinderGeometry(r, r, h, seg), m);
      c.position.set(x, y, z);
      c.castShadow = c.receiveShadow = true;
      parent.add(c);
      return c;
    }
    box(4.4, 0.2, 3.0, M.stone, 0, 0.1, 0);
    box(4.1, 0.05, 2.7, M.stoneTop, 0, 0.225, 0);
    const floor = 0.25;

    // The products, back left, and the stores they write to, back right.
    box(0.42, 0.42, 0.42, M.pipeline, -1.55, floor + 0.21, -0.85);
    box(0.42, 0.42, 0.42, M.pod, -0.95, floor + 0.21, -0.85);
    const stores = [0.75, 1.4].map((x) => {
      box(0.5, 0.58, 0.46, M.store, x, floor + 0.29, -0.85);
      box(0.54, 0.04, 0.5, M.storeTop, x, floor + 0.6, -0.85);
      box(0.3, 0.012, 0.06, M.slot, x, floor + 0.626, -0.85).castShadow = false;
      return x;
    });
    const batches = stores.map(() => {
      const b = box(0.24, 0.035, 0.16, M.batch, -1.25, floor + 0.5, -0.85);
      b.visible = false;
      return b;
    });

    // The record, front centre: a sheet on a board, five rows that fill in, and a place for the seal.
    box(1.75, 0.06, 1.15, M.board, -0.1, floor + 0.03, 0.55);
    box(1.6, 0.015, 1.0, M.paper, -0.1, floor + 0.068, 0.55);
    const rowY = floor + 0.08, x0 = -0.8;
    const lengths = [1.0, 0.6, 1.05, 0.3, 0.22];
    const tones = [M.row, M.row, M.accent, M.row, M.warn];
    const rows = lengths.map((len, i) => {
      const g = new Group();
      g.position.set(x0, rowY, 0.2 + i * 0.16);
      box(len, 0.012, 0.07, tones[i], len / 2, 0, 0, g).castShadow = false;
      root.add(g);
      return g;
    });
    const seal = cyl(0.13, 0.012, M.accent, 0.42, rowY + 0.002, 0.86);
    seal.castShadow = false;
    const stamp = new Group();
    cyl(0.15, 0.08, M.accent, 0, 0.04, 0, stamp);
    cyl(0.06, 0.26, M.handle, 0, 0.21, 0, stamp);
    cyl(0.1, 0.09, M.handle, 0, 0.38, 0, stamp);
    stamp.position.set(0.42, rowY + 0.62, 0.86);
    root.add(stamp);

    const ease = (t) => t * t * (3 - 2 * t);
    const clamp = (v) => Math.min(1, Math.max(0, v));
    return {
      root,
      pose(t) {
        // Each batch arcs from between the products into its store's slot, then sinks in.
        batches.forEach((b, i) => {
          const u = clamp((t - i * SEND) / SEND);
          b.visible = u > 0 && u < 1;
          const k = ease(u), x = -1.25 + (stores[i] + 1.25) * Math.min(1, k / 0.8);
          const y = floor + 0.5 + Math.sin(Math.min(1, k / 0.8) * Math.PI) * 0.35 - Math.max(0, (k - 0.8) / 0.2) * 0.3;
          b.position.set(x, y, -0.85);
        });
        rows.forEach((r, i) => { r.scale.x = Math.max(0.001, ease(clamp((t - B0 - 0.2 - i * FILL) / FILL))); });
        const down = ease(clamp((t - B1 - 0.15) / PRESS)), up = ease(clamp((t - B1 - 0.15 - PRESS) / LIFT));
        stamp.position.y = rowY + 0.62 - 0.6 * down + 0.6 * up;
        seal.visible = down >= 1;
      },
    };
  },
};
