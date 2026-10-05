// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Me, Span } from "../types";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({
  ...(await orig<typeof import("../api")>()),
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: vi.fn(),
}));
import { MAX_EVIDENCE } from "./Panels";
import { RunViewPage } from "./RunViewPage";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const me: Me = { principal: "prn_human01", kind: "human", email: null, via: "session" };
const API = "/v1/runs/superpipeline/brd_01/run_01";
const doc = {
  run: { ref: "superpipeline:brd_01/run_01", agent: "prn_agent01", state: "running", started_at: "2026-10-04T10:00:00Z", ended_at: null, card: { title: "A run" } },
  attempts: [{ id: "attempt_a1" }],
  errors: [],
  cost: { status: "unreported" },
  verdicts: [{ id: "gate_1", kind: "gate", standard: "gate:review", status: "approved", value: { decision: "pass" }, judge: "prn_human02" }],
  sources: { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "ok", errors: "ok", verdicts: "ok" },
};
const span = (i: number): Span => ({ trace_id: "t", span_id: `s${i}`, name: `span ${i}`, service: "svc", start: "2026-10-04T10:00:00Z", duration_ms: 5, attributes: {} });
const hv = (id: string, over: object) => ({
  id, kind: "review", subject_kind: "run", subject_ref: "superpipeline:brd_01/run_01", judge: "prn_human01", judge_kind: "human",
  standard: "rubric:release@1", value: { decision: "pass" }, comment: "", evidence_refs: [], supersedes: null, superseded_by: null, created_at: "2026-10-06T10:00:00Z", ...over,
});

let root: Root;
let host: HTMLElement;
const flush = () => act(async () => {});
const render = async (query = "", d: object = doc) => {
  getJSON.mockImplementation(async (path: string) => {
    if (path === API) return d;
    if (path.startsWith(`${API}/spans`)) return path.includes("cursor=c2") ? { spans: [span(500)], next_cursor: undefined } : { spans: Array.from({ length: 500 }, (_, i) => span(i)), next_cursor: "c2" };
    if (path.startsWith("/v1/verdicts?subject_kind=run")) return { verdicts: [hv("vrd_1", { superseded_by: "vrd_2" }), hv("vrd_2", { supersedes: "vrd_1" })] };
    if (path.startsWith("/v1/verdicts?subject_kind=attempt")) return { verdicts: [hv("vrd_3", { subject_kind: "attempt", subject_ref: "attempt_a1", judge: "prn_human02" })] };
    return {};
  });
  await act(async () => root.render(<RunViewPage board="brd_01" run="run_01" query={query} me={me} />));
  await flush();
  await flush();
};

beforeEach(() => {
  getJSON.mockReset();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

describe("RunViewPage", () => {
  it("opens the tab named in the URL", async () => {
    await render("tab=attempts");
    expect(host.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toBe("Attempts");
    expect(host.textContent).toContain("attempt_a1");
  });

  it("says a tab's source is unavailable instead of showing it empty", async () => {
    await render("tab=logs", { ...doc, sources: { ...doc.sources, logs: "timeout" } });
    expect(host.textContent).toContain("unavailable: the logs source did not answer (timeout)");
  });

  it("pages spans 500 at a time with Load more", async () => {
    await render();
    expect(host.querySelectorAll(".waterfall li")).toHaveLength(500);
    expect(getJSON.mock.calls.some(([p]) => String(p).includes("limit=500"))).toBe(true);
    const more = [...host.querySelectorAll("button")].find((b) => b.textContent === "Load more spans")!;
    await act(async () => void more.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();
    expect(host.querySelectorAll(".waterfall li")).toHaveLength(501);
    expect([...host.querySelectorAll("button")].some((b) => b.textContent === "Load more spans")).toBe(false);
  });

  it("stops offering unticked spans once 100 are ticked", async () => {
    await render();
    const boxes = () => [...host.querySelectorAll<HTMLInputElement>('.waterfall input[type="checkbox"]')];
    for (let i = 0; i < MAX_EVIDENCE; i++) await act(async () => void boxes()[i].click());
    expect(boxes().filter((b) => b.checked)).toHaveLength(MAX_EVIDENCE);
    expect(boxes()[MAX_EVIDENCE].disabled).toBe(true);
    expect(boxes()[0].disabled).toBe(false);
  });

  it("reads verdicts for the run and for each attempt, beside the gate decisions", async () => {
    await render("tab=verdicts");
    const paths = getJSON.mock.calls.map(([p]) => String(p));
    expect(paths).toContain("/v1/verdicts?subject_kind=run&subject_ref=superpipeline%3Abrd_01%2Frun_01");
    expect(paths).toContain("/v1/verdicts?subject_kind=attempt&subject_ref=attempt_a1");
    expect(host.textContent).toContain("gate:review");
    expect(host.querySelector("#vrd_1")?.className).toBe("verdict superseded");
    expect(host.querySelector("#vrd_3")).not.toBeNull();
    // Revise: only on the viewer's own, unsuperseded verdict (vrd_2); not vrd_1 (superseded), not vrd_3 (someone else's).
    const revise = [...host.querySelectorAll("button")].filter((b) => b.textContent === "Revise");
    expect(revise).toHaveLength(1);
    expect(revise[0].closest("li")?.id).toBe("vrd_2");
  });
});
