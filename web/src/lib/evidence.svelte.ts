import { refKey } from "./run/transcript";
import type { EvidenceRef } from "./types";

// A verdict cites at most this many spans and steps together.
export const MAX_EVIDENCE = 100;

export interface Evidence {
  // Everything cited, in the order it was cited: span ids and session ranges.
  readonly refs: EvidenceRef[];
  // The ticked span ids.
  readonly ticked: Set<string>;
  // The cited session ranges.
  readonly cited: EvidenceRef[];
  readonly count: number;
  // false once MAX_EVIDENCE are cited: nothing more can be added, things can still be taken away.
  readonly canAdd: boolean;
  has(ref: EvidenceRef): boolean;
  // toggle adds or removes ref; it answers false when adding was refused (the cap).
  toggle(ref: EvidenceRef): boolean;
}

// createEvidence is one run view's evidence. The run view is keyed by board/run, so a new run gets a
// new, empty one: nothing ticked on one run can reach another. It is client state, lost on reload.
export function createEvidence(): Evidence {
  let refs = $state.raw<EvidenceRef[]>([]);
  return {
    get refs() {
      return refs;
    },
    get ticked() {
      return new Set(refs.filter((r): r is string => typeof r === "string"));
    },
    get cited() {
      return refs.filter((r) => typeof r !== "string");
    },
    get count() {
      return refs.length;
    },
    get canAdd() {
      return refs.length < MAX_EVIDENCE;
    },
    has(ref) {
      const k = refKey(ref);
      return refs.some((r) => refKey(r) === k);
    },
    toggle(ref) {
      const k = refKey(ref);
      if (refs.some((r) => refKey(r) === k)) {
        refs = refs.filter((r) => refKey(r) !== k);
        return true;
      }
      if (refs.length >= MAX_EVIDENCE) return false;
      refs = [...refs, ref];
      return true;
    },
  };
}
