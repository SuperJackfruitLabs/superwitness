import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Unavailable, VerdictList } from "./Panels";
import { Facts, Tabs } from "./RunViewPage";
import type { Doc, HistoryVerdict, Me } from "../types";

describe("tabs", () => {
  it("marks the tab from the URL as selected, and puts each tab in the URL", () => {
    const html = renderToStaticMarkup(<Tabs current="logs" base="/runs/superpipeline/brd_01/run_01" />);
    expect(html).toContain('href="/runs/superpipeline/brd_01/run_01?tab=logs" role="tab" aria-selected="true"');
    expect(html).toContain('href="/runs/superpipeline/brd_01/run_01" role="tab" aria-selected="false"');
  });
  it("says a source that did not answer is unavailable", () => {
    expect(renderToStaticMarkup(<Unavailable source="traces" status="timeout" />)).toContain("unavailable: the traces source did not answer (timeout)");
  });
  it("shows unknown facts as unknown", () => {
    const html = renderToStaticMarkup(<Facts doc={{ run: {}, sources: {}, cost: {} }} />);
    expect(html).not.toContain("<dd>0</dd>");
    expect(html).toContain("<dd>unknown</dd>");
  });
});

describe("verdict history", () => {
  const me: Me = { principal: "prn_human01", kind: "human", email: null, via: "session" };
  const v = (id: string, over: Partial<HistoryVerdict>): HistoryVerdict => ({
    id, kind: "review", subject_kind: "run", subject_ref: "superpipeline:brd_01/run_01", judge: "prn_human01", judge_kind: "human",
    standard: "rubric:release@1", value: { decision: "pass" }, comment: "", evidence_refs: [], supersedes: null, superseded_by: null,
    created_at: "2026-10-06T10:00:00Z", ...over,
  });
  it("strikes through a superseded verdict, links its successor, and offers Revise only on the viewer's latest", () => {
    const html = renderToStaticMarkup(
      <VerdictList
        verdicts={[v("vrd_1", { superseded_by: "vrd_2" }), v("vrd_2", { supersedes: "vrd_1", value: { decision: "fail" } }), v("vrd_3", { judge: "prn_human02" })]}
        gates={[]}
        me={me}
        onRevise={() => {}}
      />,
    );
    expect(html).toContain('<li id="vrd_1" class="verdict superseded">');
    expect(html).toContain('<a href="#vrd_2">superseded by vrd_2</a>');
    expect(html.match(/Revise/g)).toHaveLength(1);
    expect(html.indexOf("Revise")).toBeGreaterThan(html.indexOf('id="vrd_2"'));
    expect(html.indexOf("Revise")).toBeLessThan(html.indexOf('id="vrd_3"'));
  });
  it("links a cited transcript range back to the Transcript at that range", () => {
    const attempts: Doc[] = [{ id: "attempt_a1", session_id: "acps_a1", seq_from: 1, seq_to: 9 }];
    const html = renderToStaticMarkup(
      <VerdictList
        verdicts={[v("vrd_9", { evidence_refs: ["aaaaaaaaaaaaaaaa", { session_id: "acps_a1", seq_from: 3, seq_to: 4 }, { session_id: "acps_zz", seq_from: 1, seq_to: 1 }] })]}
        gates={[]}
        me={me}
        onRevise={() => {}}
        attempts={attempts}
        page="/runs/superpipeline/brd_01/run_01"
      />,
    );
    expect(html).toContain('href="/runs/superpipeline/brd_01/run_01?tab=transcript&amp;attempt=attempt_a1&amp;seq=3-4"');
    expect(html).toContain("transcript 3\u20134 \u2197");
    expect(html.match(/transcript \d/g)).toHaveLength(1); // a range in another run's session has no link here
  });
});
