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
import { LogsPanel, MAX_EVIDENCE, TracePanel } from "./Panels";
import { ApiError } from "../api";
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
  }, 30_000);

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

describe("failures and races", () => {
  const deferred = <T,>() => {
    let resolve!: (v: T) => void;
    let reject!: (e: unknown) => void;
    const promise = new Promise<T>((a, b) => ((resolve = a), (reject = b)));
    return { promise, resolve, reject };
  };
  const click = (el: Element) => act(async () => void el.dispatchEvent(new MouseEvent("click", { bubbles: true })));
  const button = (name: string) => [...host.querySelectorAll("button")].find((b) => b.textContent === name)!;
  const view = (board = "brd_01", run = "run_01", query = "") => (
    <RunViewPage board={board} run={run} query={query} me={me} />
  );
  const docFor = (run: string) => ({ ...doc, run: { ...doc.run, ref: `superpipeline:brd_01/${run}` } });

  it("a failed verdict read says unavailable and offers a retry, never 'No verdicts yet'", async () => {
    let fail = true;
    getJSON.mockImplementation(async (path: string) => {
      if (path === API) return { ...doc, verdicts: [] };
      if (path.startsWith("/v1/verdicts?subject_kind=attempt")) return { verdicts: [hv("vrd_3", { subject_kind: "attempt", subject_ref: "attempt_a1" })] };
      if (path.startsWith("/v1/verdicts?subject_kind=run")) {
        if (fail) throw new ApiError(503, "store_unavailable", "Can't reach the database", true);
        return { verdicts: [hv("vrd_1", {})] };
      }
      return {};
    });
    await act(async () => root.render(view("brd_01", "run_01", "tab=verdicts")));
    await flush();
    await flush();
    expect(host.textContent).toContain("unavailable: the verdict history for this run could not be read (Can't reach the database)");
    expect(host.textContent).not.toContain("No verdicts yet");
    expect(host.querySelector("#vrd_3")).not.toBeNull(); // the other subject's verdicts still show
    fail = false;
    await click(button("Try again"));
    await flush();
    expect(host.querySelector("#vrd_1")).not.toBeNull();
    expect(host.textContent).not.toContain("unavailable: the verdict history");
  });

  it("only failed subjects are unavailable, and nothing else claims empty", async () => {
    getJSON.mockImplementation(async (path: string) => {
      if (path === API) return { ...doc, verdicts: [] };
      throw new ApiError(0, "network", "superwitness could not be reached");
    });
    await act(async () => root.render(view("brd_01", "run_01", "tab=verdicts")));
    await flush();
    await flush();
    expect(host.textContent).toContain("for this run");
    expect(host.textContent).toContain("for attempt attempt_a1");
    expect(host.textContent).not.toContain("No verdicts yet");
  });

  it("drops a trace response that arrives after the run changed", async () => {
    const slow = deferred<{ spans: Span[] }>();
    getJSON.mockImplementation((path: string) => {
      if (path === API) return Promise.resolve(docFor("run_01"));
      if (path === "/v1/runs/superpipeline/brd_01/run_02") return Promise.resolve(docFor("run_02"));
      if (path.startsWith(`${API}/spans`)) return slow.promise;
      return Promise.resolve({ spans: [{ ...span(1), name: "from run two" }] });
    });
    await act(async () => root.render(view()));
    await flush();
    await act(async () => root.render(view("brd_01", "run_02")));
    await flush();
    await flush();
    expect(host.textContent).toContain("from run two");
    await act(async () => slow.resolve({ spans: [{ ...span(2), name: "from run one" }] }));
    expect(host.textContent).not.toContain("from run one");
    expect(host.textContent).toContain("from run two");
  });

  it("drops a load-more that was in flight when the run changed", async () => {
    const more = deferred<{ spans: Span[] }>();
    getJSON.mockImplementation((path: string) => {
      if (path === API) return Promise.resolve(docFor("run_01"));
      if (path === "/v1/runs/superpipeline/brd_01/run_02") return Promise.resolve(docFor("run_02"));
      if (path.startsWith(`${API}/spans`)) return path.includes("cursor=") ? more.promise : Promise.resolve({ spans: [span(1)], next_cursor: "c2" });
      return Promise.resolve({ spans: [{ ...span(7), name: "from run two" }] });
    });
    await act(async () => root.render(view()));
    await flush();
    await click(button("Load more spans"));
    await act(async () => root.render(view("brd_01", "run_02")));
    await flush();
    await flush();
    await act(async () => more.resolve({ spans: [{ ...span(9), name: "late page" }] }));
    expect(host.textContent).not.toContain("late page");
    expect(host.querySelectorAll(".waterfall li")).toHaveLength(1);
  });

  it("a failed load-more keeps the loaded spans and shows the error beside the button", async () => {
    let fail = true;
    getJSON.mockImplementation(async (path: string) => {
      if (path === API) return doc;
      if (path.includes("cursor=")) {
        if (fail) throw new ApiError(503, "x", "The trace source is busy", true);
        return { spans: [span(2)] };
      }
      return { spans: [span(1)], next_cursor: "c2" };
    });
    await act(async () => root.render(view()));
    await flush();
    await click(button("Load more spans"));
    await flush();
    expect(host.querySelectorAll(".waterfall li")).toHaveLength(1);
    expect(host.querySelector(".refusal")?.textContent).toBe("The trace source is busy");
    fail = false;
    await click(button("Try loading more spans again"));
    await flush();
    expect(host.querySelectorAll(".waterfall li")).toHaveLength(2);
    expect(host.querySelector(".refusal")).toBeNull();
  });

  it("a trace error clears on retry", async () => {
    let fail = true;
    getJSON.mockImplementation(async (path: string) => {
      if (path === API) return doc;
      if (fail) throw new ApiError(500, "x", "boom");
      return { spans: [span(1)] };
    });
    await act(async () => root.render(view()));
    await flush();
    expect(host.querySelector(".refusal")?.textContent).toBe("boom");
    fail = false;
    await click(button("Try again"));
    await flush();
    expect(host.querySelector(".refusal")).toBeNull();
    expect(host.querySelectorAll(".waterfall li")).toHaveLength(1);
  });

  it("drops a log response that arrives after the level changed", async () => {
    const slow = deferred<{ logs: object[]; next_cursor: null }>();
    const line = (m: string) => ({ at: "t", service: "s", level: "info", message: m });
    getJSON.mockImplementation((path: string) => {
      if (path === API) return Promise.resolve(doc);
      if (path.includes("level=error")) return Promise.resolve({ logs: [line("an error line")], next_cursor: null });
      return slow.promise;
    });
    await act(async () => root.render(view("brd_01", "run_01", "tab=logs")));
    await flush();
    const sel = host.querySelector("select")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(sel, "error");
      sel.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    expect(host.textContent).toContain("an error line");
    await act(async () => slow.resolve({ logs: [line("an unfiltered line")], next_cursor: null }));
    expect(host.textContent).not.toContain("an unfiltered line");
    expect(host.textContent).toContain("an error line");
  });

  it("drops a log load-more that was in flight when the level changed", async () => {
    const more = deferred<{ logs: object[]; next_cursor: null }>();
    const line = (m: string) => ({ at: "t", service: "s", level: "info", message: m });
    getJSON.mockImplementation((path: string) => {
      if (path === API) return Promise.resolve(doc);
      if (path.includes("cursor=")) return more.promise;
      if (path.includes("level=error")) return Promise.resolve({ logs: [line("an error line")], next_cursor: null });
      return Promise.resolve({ logs: [line("first")], next_cursor: "c2" });
    });
    await act(async () => root.render(view("brd_01", "run_01", "tab=logs")));
    await flush();
    await click(button("Load more"));
    const sel = host.querySelector("select")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(sel, "error");
      sel.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    await act(async () => more.resolve({ logs: [line("late log")], next_cursor: null }));
    expect(host.textContent).not.toContain("late log");
    expect(host.textContent).toContain("an error line");
  });

  it("forgets ticked evidence when the run changes", async () => {
    getJSON.mockImplementation(async (path: string) => {
      if (path.startsWith("/v1/runs/") && !path.includes("/spans")) return doc;
      return { spans: [span(1)] };
    });
    await act(async () => root.render(view()));
    await flush();
    await act(async () => void host.querySelector<HTMLInputElement>('.waterfall input[type="checkbox"]')!.click());
    expect(host.textContent).toContain("(1 of at most");
    await act(async () => root.render(view("brd_01", "run_02")));
    await flush();
    await flush();
    expect(host.textContent).toContain("(0 of at most");
  });

  // The panels are exercised directly here: keyed by run, the page would remount them and hide a
  // missing generation check.
  it("TracePanel drops a first-load and a load-more response from the previous base", async () => {
    const first = deferred<{ spans: Span[] }>();
    const more = deferred<{ spans: Span[] }>();
    getJSON.mockImplementation((path: string) => {
      if (path.startsWith("/a/spans")) return path.includes("cursor=") ? more.promise : Promise.resolve({ spans: [{ ...span(1), name: "a one" }], next_cursor: "c2" });
      if (path.startsWith("/b/spans")) return Promise.resolve({ spans: [{ ...span(3), name: "b one" }] });
      return first.promise;
    });
    const panel = (base: string) => <TracePanel base={base} evidence={[]} setEvidence={() => {}} />;
    await act(async () => root.render(panel("/a")));
    await flush();
    await click(button("Load more spans"));
    await act(async () => root.render(panel("/b")));
    await flush();
    await act(async () => more.resolve({ spans: [{ ...span(2), name: "a late" }] }));
    expect(host.textContent).not.toContain("a late");
    expect(host.textContent).toContain("b one");
    expect(host.textContent).not.toContain("a one");
  });

  it("TracePanel drops a slow first load from the previous base", async () => {
    const slow = deferred<{ spans: Span[] }>();
    getJSON.mockImplementation((path: string) => (path.startsWith("/a/") ? slow.promise : Promise.resolve({ spans: [{ ...span(3), name: "b one" }] })));
    const panel = (base: string) => <TracePanel base={base} evidence={[]} setEvidence={() => {}} />;
    await act(async () => root.render(panel("/a")));
    await act(async () => root.render(panel("/b")));
    await flush();
    await act(async () => slow.resolve({ spans: [{ ...span(2), name: "a slow" }] }));
    expect(host.textContent).not.toContain("a slow");
    expect(host.textContent).toContain("b one");
  });

  it("TracePanel clears the previous base's error when the base changes", async () => {
    getJSON.mockImplementation(async (path: string) => {
      if (path.startsWith("/a/")) throw new ApiError(500, "x", "a failed");
      return { spans: [{ ...span(3), name: "b one" }] };
    });
    const panel = (base: string) => <TracePanel base={base} evidence={[]} setEvidence={() => {}} />;
    await act(async () => root.render(panel("/a")));
    await flush();
    expect(host.textContent).toContain("a failed");
    await act(async () => root.render(panel("/b")));
    await flush();
    expect(host.textContent).not.toContain("a failed");
    expect(host.textContent).toContain("b one");
  });

  it("LogsPanel drops a response from the previous base", async () => {
    const slow = deferred<{ logs: object[]; next_cursor: null }>();
    getJSON.mockImplementation((path: string) =>
      path.startsWith("/a/") ? slow.promise : Promise.resolve({ logs: [{ at: "t", service: "s", level: "info", message: "b line" }], next_cursor: null }),
    );
    await act(async () => root.render(<LogsPanel base="/a" />));
    await act(async () => root.render(<LogsPanel base="/b" />));
    await flush();
    await act(async () => slow.resolve({ logs: [{ at: "t", service: "s", level: "info", message: "a line" }], next_cursor: null }));
    expect(host.textContent).not.toContain("a line");
    expect(host.textContent).toContain("b line");
  });
});
