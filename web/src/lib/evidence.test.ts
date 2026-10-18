import { describe, expect, it } from "vitest";
import { createEvidence, MAX_EVIDENCE } from "./evidence.svelte";

describe("evidence", () => {
  it("ticks and unticks span ids and cites ranges by value", () => {
    const e = createEvidence();
    expect(e.toggle("s1")).toBe(true);
    expect(e.toggle({ session_id: "acps_a1", seq_from: 3, seq_to: 4 })).toBe(true);
    expect(e.count).toBe(2);
    expect(e.has("s1")).toBe(true);
    expect(e.has({ seq_to: 4, seq_from: 3, session_id: "acps_a1" } as never)).toBe(true);
    expect([...e.ticked]).toEqual(["s1"]);
    expect(e.cited).toHaveLength(1);
    e.toggle("s1");
    expect(e.has("s1")).toBe(false);
    expect(e.count).toBe(1);
  });

  it("refuses the 101st, and still lets one be taken away", () => {
    const e = createEvidence();
    for (let i = 0; i < MAX_EVIDENCE; i++) expect(e.toggle(`s${i}`)).toBe(true);
    expect(MAX_EVIDENCE).toBe(100);
    expect(e.canAdd).toBe(false);
    expect(e.toggle("s100")).toBe(false);
    expect(e.count).toBe(100);
    expect(e.has("s100")).toBe(false);
    expect(e.toggle("s0")).toBe(true);
    expect(e.count).toBe(99);
    expect(e.canAdd).toBe(true);
  });

  it("keeps the order things were cited in", () => {
    const e = createEvidence();
    e.toggle("b");
    e.toggle("a");
    expect(e.refs).toEqual(["b", "a"]);
  });
});
