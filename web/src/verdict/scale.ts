import type { Scale } from "../types";

// The drawer's input for each recognised scale, and the verdict value it sends.
export type Input =
  | { kind: "decision" | "label"; choice: string | null }
  | { kind: "score"; x: number }
  | { kind: "text"; text: string };

export const range = (s: Scale) => ({ lo: s.min ?? 0, hi: s.max ?? 1 });

// A score slider has twenty steps: 0.05 on the 0 to 1 scale.
export const stepOf = (s: Scale) => {
  const { lo, hi } = range(s);
  return (hi - lo) / 20;
};

// initialInput is empty for a new verdict and the earlier value for a revision.
export function initialInput(s: Scale, prev?: Record<string, unknown>): Input {
  switch (s.kind) {
    case "decision":
      return { kind: "decision", choice: typeof prev?.decision === "string" ? prev.decision : null };
    case "label":
      return { kind: "label", choice: typeof prev?.label === "string" ? prev.label : null };
    case "text":
      return { kind: "text", text: typeof prev?.text === "string" ? prev.text : "" };
    case "score": {
      const { lo, hi } = range(s);
      const p = typeof prev?.score === "number" ? prev.score : 0.5;
      return { kind: "score", x: lo + p * (hi - lo) };
    }
  }
}

// valueOf is what POST /v1/verdicts receives, or null while the input is incomplete. A score on
// a legacy {min, max} scale is rescaled to 0 to 1.
export function valueOf(s: Scale, i: Input): Record<string, unknown> | null {
  switch (i.kind) {
    case "decision":
      return i.choice ? { decision: i.choice } : null;
    case "label":
      return i.choice ? { label: i.choice } : null;
    case "text":
      return i.text.trim() ? { text: i.text } : null;
    case "score": {
      const { lo, hi } = range(s);
      const p = Math.min(1, Math.max(0, (i.x - lo) / (hi - lo)));
      return { score: Math.round(p * 10000) / 10000 };
    }
  }
}
