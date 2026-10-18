import { refKey } from "../run/transcript";
import type { EvidenceRef, HistoryVerdict } from "../types";

export const MAX_COMMENT = 10000;

// citedEvidence is what a verdict cites: when revising, the earlier verdict's evidence first, then
// any spans ticked and steps cited now, without repeats. More than MAX_EVIDENCE blocks the save.
export function citedEvidence(ticked: EvidenceRef[], revising?: HistoryVerdict): unknown[] {
  const out: unknown[] = [];
  const seen = new Set<string>();
  const earlier: unknown[] = Array.isArray(revising?.evidence_refs) ? revising.evidence_refs : [];
  for (const ref of [...earlier, ...ticked]) {
    const k = refKey(ref);
    if (!seen.has(k)) {
      seen.add(k);
      out.push(ref);
    }
  }
  return out;
}

// evidenceSummary is the drawer's evidence line: what is cited, and where it came from.
export function evidenceSummary(evidence: unknown[], kept: number): string {
  if (evidence.length === 0) return "none; tick spans in the Trace tab or cite steps in the Transcript tab";
  const added = evidence.slice(kept);
  const spans = added.filter((r) => typeof r === "string").length;
  const steps = added.length - spans;
  const stepText = `${steps} step${steps === 1 ? "" : "s"} from the transcript`;
  const parts = [...(spans > 0 ? [`${spans} ticked in the Trace tab`] : []), ...(steps > 0 ? [stepText] : [])];
  if (kept > 0) return [`${evidence.length} cited: ${kept} from the verdict being revised`, ...parts].join(", ");
  if (steps === 0) return `${spans} span${spans === 1 ? "" : "s"} ticked in the Trace tab`;
  return `${evidence.length} cited: ${parts.join(", ")}`;
}
