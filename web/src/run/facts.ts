import { duration, list } from "../format";
import type { Doc, Span } from "../types";

export const TABS = ["trace", "logs", "errors", "verdicts", "attempts"] as const;
export type Tab = (typeof TABS)[number];

export const TAB_LABEL: Record<Tab, string> = { trace: "Trace", logs: "Logs", errors: "Errors", verdicts: "Verdicts", attempts: "Attempts" };

// The source each tab reads; when it did not answer, the tab says so instead of showing nothing.
export const TAB_SOURCE: Record<Tab, string> = { trace: "traces", logs: "logs", errors: "errors", verdicts: "verdicts", attempts: "agentpod" };

export function tabOf(v: string | null): Tab {
  return (TABS as readonly string[]).includes(v ?? "") ? (v as Tab) : "trace";
}

// facts is the strip under a run's title. A value a source could not supply is "unknown", never 0.
export function facts(doc: Doc): { label: string; value: string }[] {
  const sources: Record<string, string> = doc.sources ?? {};
  const run = doc.run ?? {};
  const ok = (s: string) => sources[s] === "ok";
  const cost = doc.cost ?? {};
  let dur = "unknown";
  if (run.started_at && run.started_at !== "unknown") {
    if (run.ended_at === null) dur = "in progress";
    else if (run.ended_at && run.ended_at !== "unknown") dur = duration(Date.parse(run.ended_at) - Date.parse(run.started_at));
  }
  let costText = "unknown";
  if (cost.status === "reported" && typeof cost.usd === "number") costText = `$${cost.usd.toFixed(2)}`;
  else if (cost.status === "unreported") costText = "unreported";
  return [
    { label: "Agent", value: run.agent ?? "unknown" },
    { label: "Attempts", value: ok("agentpod") ? String(list(doc.attempts).length) : "unknown" },
    { label: "Errors", value: ok("errors") ? String(list(doc.errors).length) : "unknown" },
    { label: "Duration", value: dur },
    { label: "Cost", value: costText },
  ];
}

export interface Row {
  span: Span;
  depth: number;
  left: number; // percent of the run's span time
  width: number;
}

// waterfall lays spans out on one time axis, each indented under its parent.
export function waterfall(spans: Span[]): Row[] {
  if (spans.length === 0) return [];
  const starts = spans.map((s) => Date.parse(s.start));
  const t0 = Math.min(...starts);
  const t1 = Math.max(...spans.map((s, i) => starts[i] + s.duration_ms));
  const total = Math.max(t1 - t0, 1);
  const byId = new Map(spans.map((s) => [s.span_id, s]));
  const depthOf = (s: Span, hops = 0): number => {
    const p = s.parent_span_id ? byId.get(s.parent_span_id) : undefined;
    return p && hops < 32 ? 1 + depthOf(p, hops + 1) : 0;
  };
  return spans
    .map((s, i) => ({ span: s, depth: depthOf(s), left: ((starts[i] - t0) / total) * 100, width: Math.max((s.duration_ms / total) * 100, 0.5) }))
    .sort((a, b) => a.left - b.left);
}
