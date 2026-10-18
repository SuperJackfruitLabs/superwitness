import type { Scale } from "./types";

// describeScale says a rubric's recognised scale in words (the rubric page's Scale section).
export function describeScale(s: Scale | null): string {
  if (!s) return "Not supported in the app.";
  switch (s.kind) {
    case "decision":
      return `A decision: ${(s.options ?? []).join(", ")}.`;
    case "label":
      return `A label: ${(s.labels ?? []).join(", ")}.`;
    case "score":
      return s.min === 0 && s.max === 1 ? "A score from 0 to 1." : `A score from ${s.min} to ${s.max}, recorded as 0 to 1.`;
    case "text":
      return "Free text.";
  }
}
