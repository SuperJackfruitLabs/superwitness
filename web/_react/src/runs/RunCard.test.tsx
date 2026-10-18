import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { RegistryRun } from "../types";
import { RunCard } from "./RunCard";

const now = new Date("2026-10-06T10:00:00Z");
const base: RegistryRun = {
  source: "superpipeline", external_ref: "brd_01/run_01", ref: "superpipeline:brd_01/run_01",
  scope: { id: "brd_01", name: "Press" }, title: "Write the release note", executor: { id: null, name: "drafter" },
  status: "succeeded", source_status: "completed", started_at: "2026-10-06T09:50:00Z", ended_at: "2026-10-06T09:54:56Z",
  reported_at: "2026-10-06T09:54:57Z", first_seen_at: "2026-10-06T09:50:01Z", updated_at: "2026-10-06T09:54:58Z",
  latest_verdict: null,
};

describe("RunCard", () => {
  it("shows the title, status, scope, executor, duration and verdict, and links to the run", () => {
    const html = renderToStaticMarkup(<RunCard run={base} now={now} />);
    for (const want of ['href="/runs/superpipeline/brd_01/run_01"', '<span class="pill good">succeeded</span>', "Write the release note", "Press", "drafter", "4m 56s", "no verdict"]) {
      expect(html).toContain(want);
    }
  });
  it("falls back to the ref, says how long ago a running run started, and shows the latest verdict", () => {
    const html = renderToStaticMarkup(
      <RunCard run={{ ...base, title: null, ended_at: null, status: "running", latest_verdict: { id: "vrd_1", judge: "prn_human01", judge_kind: "human", standard: "rubric:release@1", value: { decision: "fail" }, created_at: "x" } }} now={now} />,
    );
    expect(html).toContain("<code>brd_01/run_01</code>");
    expect(html).toContain("started 10 min ago");
    expect(html).toContain("verdict: fail");
  });
  it("does not link a run from a source without a run page", () => {
    const html = renderToStaticMarkup(<RunCard run={{ ...base, source: "canary", external_ref: "x/1" }} now={now} />);
    expect(html).not.toContain("href=");
  });
  it("says unknown for a missing executor name, never an empty or zero value", () => {
    const html = renderToStaticMarkup(<RunCard run={{ ...base, executor: { id: null, name: null } }} now={now} />);
    expect(html).toContain("<span>unknown</span>");
  });
});
