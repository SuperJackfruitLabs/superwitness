import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/svelte";
import RunCard, { runHref } from "./RunCard.svelte";
import type { RegistryRun } from "../types";

afterEach(cleanup);

const now = new Date("2026-10-06T10:00:00Z");
const base: RegistryRun = {
  source: "superpipeline", external_ref: "brd_01/run_01", ref: "superpipeline:brd_01/run_01",
  scope: { id: "brd_01", name: "Press" }, title: "Write the release note", executor: { id: null, name: "drafter" },
  status: "succeeded", source_status: "completed", started_at: "2026-10-06T09:50:00Z", ended_at: "2026-10-06T09:54:56Z",
  reported_at: "2026-10-06T09:54:57Z", first_seen_at: "2026-10-06T09:50:01Z", updated_at: "2026-10-06T09:54:58Z",
  latest_verdict: null,
};
const card = (run: RegistryRun) => render(RunCard, { run, now }).container;
const verdict = (value: Record<string, unknown>) => ({ id: "vrd_1", judge: "prn_human01", judge_kind: "human", standard: "rubric:release@1", value, created_at: "x" });

describe("RunCard", () => {
  it("shows the title, status, scope, executor, duration and verdict, and links to the run", () => {
    const c = card(base);
    const a = c.querySelector("a")!;
    expect(a).toHaveAttribute("href", "/runs/superpipeline/brd_01/run_01");
    expect(c.querySelector(".sw-pill")).toHaveAttribute("data-tone", "success");
    expect(c.querySelector(".sw-pill")!.textContent).toBe("succeeded");
    for (const want of ["Write the release note", "Press", "drafter", "4m 56s", "no verdict"]) expect(c.textContent).toContain(want);
  });
  it("falls back to the ref, says how long ago a running run started, and shows the latest verdict", () => {
    const c = card({ ...base, title: null, ended_at: null, status: "running", latest_verdict: verdict({ decision: "fail" }) });
    expect(c.querySelector("code")!.textContent).toBe("brd_01/run_01");
    expect(c.textContent).toContain("started 10 min ago");
    expect(c.textContent).toContain("verdict: fail");
  });
  it("does not link a run from a source without a run page, and says so", () => {
    const c = card({ ...base, source: "canary", external_ref: "x/1" });
    expect(c.querySelector("a")).toBeNull();
    expect(c.querySelector("[href]")).toBeNull();
    expect(c.querySelector('[title="This source has no run page yet"]')).not.toBeNull();
    expect(c.textContent).toContain("no run page yet");
  });
  it("links only a superpipeline ref shaped board/run", () => {
    expect(runHref({ ...base, external_ref: "brd_01" })).toBeNull();
    expect(runHref({ ...base, external_ref: "a/b/c" })).toBeNull();
    expect(runHref({ ...base, source: "agentpod" })).toBeNull();
    expect(runHref(base)).toBe("/runs/superpipeline/brd_01/run_01");
  });
  it("says unknown for a missing executor name, never an empty or zero value", () => {
    const c = card({ ...base, executor: { id: null, name: null } });
    expect([...c.querySelectorAll(".sw-card__meta span")].map((s) => s.textContent)).toContain("unknown");
  });
  it("falls back to the scope id, and leaves out a missing scope or executor", () => {
    expect(card({ ...base, scope: { id: "brd_01", name: null } }).textContent).toContain("brd_01");
    cleanup();
    const c = card({ ...base, scope: null, executor: null });
    expect(c.querySelectorAll(".sw-card__meta > span")).toHaveLength(2);
  });
  it("says first seen when a run has not started", () => {
    expect(card({ ...base, started_at: null, ended_at: null }).textContent).toContain("first seen 10 min ago");
  });
  it.each([
    [{ label: "formal" }, "verdict: formal"],
    [{ score: 0.8 }, "verdict: 0.80"],
    [{ text: "x".repeat(50) }, `verdict: ${"x".repeat(39)}…`],
    [{ other: 1 }, 'verdict: {"other":1}'],
  ])("says the verdict %j as %s", (value, want) => {
    expect(card({ ...base, latest_verdict: verdict(value) }).textContent).toContain(want);
  });
});
