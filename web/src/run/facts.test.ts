import { describe, expect, it } from "vitest";
import type { Span } from "../types";
import { facts, tabOf, waterfall } from "./facts";

describe("facts", () => {
  it("says unknown, never 0, for what a source could not supply", () => {
    const f = facts({ run: { agent: "prn_agent01", started_at: "unknown", ended_at: "unknown" }, attempts: [], errors: [],
      cost: { status: "unknown" }, sources: { agentpod: "timeout", errors: "unavailable" } });
    expect(f.map((x) => x.value)).toEqual(["prn_agent01", "unknown", "unknown", "unknown", "unknown"]);
  });
  it("shows what the sources supplied", () => {
    const f = facts({ run: { agent: "prn_agent01", started_at: "2026-10-04T10:00:00Z", ended_at: "2026-10-04T10:05:00Z" },
      attempts: [{}], errors: [], cost: { status: "reported", usd: 0.4 }, sources: { agentpod: "ok", errors: "ok" } });
    expect(f.map((x) => x.value)).toEqual(["prn_agent01", "1", "0", "5m 0s", "$0.40"]);
    expect(facts({ run: { started_at: "2026-10-04T10:00:00Z", ended_at: null }, cost: { status: "unreported" }, sources: {} })
      .map((x) => x.value).slice(3)).toEqual(["in progress", "unreported"]);
  });
});

describe("tabOf", () => {
  it("defaults to trace", () => {
    expect(tabOf(null)).toBe("trace");
    expect(tabOf("logs")).toBe("logs");
    expect(tabOf("bogus")).toBe("trace");
  });
});

describe("waterfall", () => {
  const span = (id: string, parent: string | undefined, start: string, ms: number): Span =>
    ({ trace_id: "t", span_id: id, parent_span_id: parent, name: id, service: "s", start, duration_ms: ms, attributes: {} });
  it("places spans on one axis and indents children", () => {
    const rows = waterfall([span("b", "a", "2026-10-04T10:00:02Z", 6000), span("a", undefined, "2026-10-04T10:00:00Z", 10000)]);
    expect(rows.map((r) => [r.span.span_id, r.depth, r.left, r.width])).toEqual([["a", 0, 0, 100], ["b", 1, 20, 60]]);
  });
  it("survives a parent cycle", () => {
    expect(waterfall([span("a", "b", "2026-10-04T10:00:00Z", 1), span("b", "a", "2026-10-04T10:00:00Z", 1)])).toHaveLength(2);
  });
});
