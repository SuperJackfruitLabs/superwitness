import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render } from "@testing-library/svelte";
import { ApiError } from "../api";
import { createEvidence, MAX_EVIDENCE } from "../evidence.svelte";
import type { Span } from "../types";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({ ...(await orig<typeof import("../api")>()), getJSON: (...a: unknown[]) => getJSON(...a) }));
const goto = vi.fn();
vi.mock("$app/navigation", () => ({ goto: (...a: unknown[]) => goto(...a) }));
import RunPage from "./RunPage.svelte";
import TracePanel from "./TracePanel.svelte";

const API = "/v1/runs/superpipeline/brd_01/run_01";
const BASE = "/runs/superpipeline/brd_01/run_01";
const doc = {
  run: { ref: "superpipeline:brd_01/run_01", agent: "prn_agent01", state: "running", started_at: "2026-10-04T10:00:00Z", ended_at: null, card: { title: "A run" } },
  attempts: [{ id: "attempt_a1" }],
  errors: [],
  cost: { status: "unreported" },
  verdicts: [],
  sources: { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "ok", errors: "ok", verdicts: "ok" },
};
const span = (i: number): Span => ({ trace_id: "t", span_id: `s${i}`, name: `span ${i}`, service: "svc", start: "2026-10-04T10:00:00Z", duration_ms: 5, attributes: {} });
const docFor = (run: string) => ({ ...doc, run: { ...doc.run, ref: `superpipeline:brd_01/${run}` } });
const deferred = <T>() => {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((a, b) => ((resolve = a), (reject = b)));
  return { promise, resolve, reject };
};

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const text = () => document.body.textContent ?? "";
const rows = () => document.querySelectorAll(".waterfall li");
const button = (name: string) => [...document.querySelectorAll("button")].find((b) => b.textContent?.trim() === name)!;
const boxes = () => [...document.querySelectorAll<HTMLInputElement>('.waterfall input[type="checkbox"]')];
const click = async (el: Element) => {
  await fireEvent.click(el);
  await flush();
};

let view: ReturnType<typeof render<typeof RunPage>>;
const page = async (query = "", run = "run_01") => {
  view = render(RunPage, { board: "brd_01", run, query });
  await flush();
  await flush();
};
const mount = async (query = "") => {
  getJSON.mockImplementation(async (path: string) => {
    if (path === API) return doc;
    if (path.startsWith(`${API}/spans`)) return path.includes("cursor=c2") ? { spans: [span(500)], next_cursor: undefined } : { spans: Array.from({ length: 500 }, (_, i) => span(i)), next_cursor: "c2" };
    return {};
  });
  await page(query);
};

beforeEach(() => {
  getJSON.mockReset();
  goto.mockReset();
});
afterEach(cleanup);

describe("the waterfall", () => {
  it("pages spans 500 at a time with Load more", async () => {
    await mount();
    expect(rows()).toHaveLength(500);
    expect(getJSON.mock.calls.some(([p]) => String(p) === `${API}/spans?limit=500`)).toBe(true);
    await click(button("Load more spans"));
    expect(getJSON.mock.calls.some(([p]) => String(p) === `${API}/spans?limit=500&cursor=c2`)).toBe(true);
    expect(rows()).toHaveLength(501);
    expect([...document.querySelectorAll("button")].some((b) => b.textContent?.trim() === "Load more spans")).toBe(false);
  });

  it("stops offering unticked spans once 100 are ticked, and the 101st is refused", async () => {
    await mount();
    for (let i = 0; i < MAX_EVIDENCE; i++) await fireEvent.click(boxes()[i]);
    await flush();
    expect(boxes().filter((b) => b.checked)).toHaveLength(MAX_EVIDENCE);
    expect(boxes()[MAX_EVIDENCE].disabled).toBe(true);
    expect(boxes()[0].disabled).toBe(false);
    expect(text()).toContain(`(${MAX_EVIDENCE} of at most ${MAX_EVIDENCE})`);
    // A click that slips through on a disabled box changes nothing.
    boxes()[MAX_EVIDENCE].disabled = false;
    await fireEvent.click(boxes()[MAX_EVIDENCE]);
    await flush();
    expect(boxes()[MAX_EVIDENCE].checked).toBe(false);
    expect(text()).toContain(`(${MAX_EVIDENCE} of at most ${MAX_EVIDENCE})`);
  }, 30_000);

  it("draws each span: cite box, indented name with its service and id, bar by share of the trace, duration", async () => {
    const spans: Span[] = [
      { trace_id: "t", span_id: "root", name: "dispatch", service: "agentpod-hub", start: "2026-10-04T10:00:00Z", duration_ms: 1000, attributes: {} },
      { trace_id: "t", span_id: "kid", parent_span_id: "root", name: "tool_call", service: "agentpod-hub", start: "2026-10-04T10:00:00.500Z", duration_ms: 250.4, attributes: { "otel.status_code": "ERROR" } },
    ];
    getJSON.mockImplementation(async (path: string) => (path === API ? doc : { spans }));
    await page();
    expect(text()).toContain("Tick spans to cite them in a verdict (0 of at most 100). A span's name opens its details.");
    const [a, b] = [...rows()];
    expect(a.querySelector("input")?.getAttribute("aria-label")).toBe("Cite dispatch as evidence");
    const name = b.querySelector<HTMLButtonElement>("button.span-name")!;
    expect(name.textContent?.trim()).toBe("tool_call");
    expect(name.getAttribute("title")).toBe("agentpod-hub · kid");
    expect(name.getAttribute("aria-pressed")).toBe("false");
    expect((b.querySelector(".sw-wf__name") as HTMLElement).style.getPropertyValue("--depth")).toBe("1");
    expect((a.querySelector(".sw-wf__name") as HTMLElement).style.getPropertyValue("--depth")).toBe("0");
    expect(b.textContent).toContain("agentpod-hub");
    expect(b.textContent).toContain("ERROR");
    const bar = b.querySelector<HTMLElement>(".bar")!;
    expect(bar.style.left).toBe("50%");
    expect(parseFloat(bar.style.width)).toBeCloseTo(25.04, 5);
    expect(b.querySelector(".dur")?.textContent).toBe("250 ms");
    expect(a.querySelector(".dur")?.textContent).toBe("1000 ms");
  });

  it("a span's name opens its details in the URL", async () => {
    getJSON.mockImplementation(async (path: string) => (path === API ? doc : { spans: [span(1)] }));
    await page();
    await click(document.querySelector("button.span-name")!);
    expect(goto.mock.calls[0][0]).toBe(`${BASE}?span=s1`);
  });

  it("says No spans. for an empty trace and Loading spans… while it is read", async () => {
    getJSON.mockImplementation(async (path: string) => (path === API ? doc : { spans: [] }));
    await page();
    expect(text()).toContain("No spans.");
    cleanup();
    getJSON.mockImplementation((path: string) => (path === API ? Promise.resolve(doc) : new Promise(() => {})));
    await page();
    expect(text()).toContain("Loading spans…");
  });

  it("a ?span= id that is not in the loaded trace opens no pane and leaves the waterfall as it is", async () => {
    getJSON.mockImplementation(async (path: string) => (path === API ? doc : { spans: [span(1)], next_cursor: "c2" }));
    await page("span=not_loaded");
    expect(document.querySelector('aside[aria-label="Span details"]')).toBeNull();
    expect(rows()).toHaveLength(1);
    expect(text()).not.toContain("not_loaded");
    expect(document.querySelector('button.span-name[aria-pressed="true"]')).toBeNull();
    expect(getJSON.mock.calls.some(([p]) => String(p).includes("/transcript") || String(p).includes("/logs"))).toBe(false);
  });
});

describe("failures and races", () => {
  it("drops a trace response that arrives after the run changed", async () => {
    const slow = deferred<{ spans: Span[] }>();
    getJSON.mockImplementation((path: string) => {
      if (path === API) return Promise.resolve(docFor("run_01"));
      if (path === "/v1/runs/superpipeline/brd_01/run_02") return Promise.resolve(docFor("run_02"));
      if (path.startsWith(`${API}/spans`)) return slow.promise;
      return Promise.resolve({ spans: [{ ...span(1), name: "from run two" }] });
    });
    await page();
    await view.rerender({ board: "brd_01", run: "run_02", query: "" });
    await flush();
    await flush();
    expect(text()).toContain("from run two");
    slow.resolve({ spans: [{ ...span(2), name: "from run one" }] });
    await flush();
    expect(text()).not.toContain("from run one");
    expect(text()).toContain("from run two");
  });

  it("drops a load-more that was in flight when the run changed", async () => {
    const more = deferred<{ spans: Span[] }>();
    getJSON.mockImplementation((path: string) => {
      if (path === API) return Promise.resolve(docFor("run_01"));
      if (path === "/v1/runs/superpipeline/brd_01/run_02") return Promise.resolve(docFor("run_02"));
      if (path.startsWith(`${API}/spans`)) return path.includes("cursor=") ? more.promise : Promise.resolve({ spans: [span(1)], next_cursor: "c2" });
      return Promise.resolve({ spans: [{ ...span(7), name: "from run two" }] });
    });
    await page();
    await click(button("Load more spans"));
    await view.rerender({ board: "brd_01", run: "run_02", query: "" });
    await flush();
    await flush();
    more.resolve({ spans: [{ ...span(9), name: "late page" }] });
    await flush();
    expect(text()).not.toContain("late page");
    expect(rows()).toHaveLength(1);
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
    await page();
    await click(button("Load more spans"));
    expect(rows()).toHaveLength(1);
    expect(document.querySelector('[role="alert"]')?.textContent).toBe("The trace source is busy");
    fail = false;
    await click(button("Try loading more spans again"));
    expect(rows()).toHaveLength(2);
    expect(document.querySelector('[role="alert"]')).toBeNull();
  });

  it("a trace error clears on retry", async () => {
    let fail = true;
    getJSON.mockImplementation(async (path: string) => {
      if (path === API) return doc;
      if (fail) throw new ApiError(500, "x", "boom");
      return { spans: [span(1)] };
    });
    await page();
    expect(document.querySelector('[role="alert"]')?.textContent).toBe("boom");
    fail = false;
    await click(button("Try again"));
    expect(document.querySelector('[role="alert"]')).toBeNull();
    expect(rows()).toHaveLength(1);
  });

  it("forgets ticked evidence when the run changes", async () => {
    getJSON.mockImplementation(async (path: string) => (path.startsWith("/v1/runs/") && !path.includes("/spans") ? doc : { spans: [span(1)] }));
    await page();
    await click(boxes()[0]);
    expect(text()).toContain("(1 of at most");
    await view.rerender({ board: "brd_01", run: "run_02", query: "" });
    await flush();
    await flush();
    expect(text()).toContain("(0 of at most");
  });
});

// The panel is exercised directly here: keyed by run, the page would remount it and hide a missing
// generation check.
describe("TracePanel on its own", () => {
  const panel = (api: string) => ({ api, page: "", attempts: [], selected: null, evidence: createEvidence() });

  it("drops a first-load and a load-more response from the previous base", async () => {
    const first = deferred<{ spans: Span[] }>();
    const more = deferred<{ spans: Span[] }>();
    getJSON.mockImplementation((path: string) => {
      if (path.startsWith("/a/spans")) return path.includes("cursor=") ? more.promise : Promise.resolve({ spans: [{ ...span(1), name: "a one" }], next_cursor: "c2" });
      if (path.startsWith("/b/spans")) return Promise.resolve({ spans: [{ ...span(3), name: "b one" }] });
      return first.promise;
    });
    const v = render(TracePanel, panel("/a"));
    await flush();
    await click(button("Load more spans"));
    await v.rerender(panel("/b"));
    await flush();
    more.resolve({ spans: [{ ...span(2), name: "a late" }] });
    await flush();
    expect(text()).not.toContain("a late");
    expect(text()).toContain("b one");
    expect(text()).not.toContain("a one");
  });

  it("drops a slow first load from the previous base", async () => {
    const slow = deferred<{ spans: Span[] }>();
    getJSON.mockImplementation((path: string) => (path.startsWith("/a/") ? slow.promise : Promise.resolve({ spans: [{ ...span(3), name: "b one" }] })));
    const v = render(TracePanel, panel("/a"));
    await v.rerender(panel("/b"));
    await flush();
    slow.resolve({ spans: [{ ...span(2), name: "a slow" }] });
    await flush();
    expect(text()).not.toContain("a slow");
    expect(text()).toContain("b one");
  });

  it("clears the previous base's error when the base changes", async () => {
    getJSON.mockImplementation(async (path: string) => {
      if (path.startsWith("/a/")) throw new ApiError(500, "x", "a failed");
      return { spans: [{ ...span(3), name: "b one" }] };
    });
    const v = render(TracePanel, panel("/a"));
    await flush();
    expect(text()).toContain("a failed");
    await v.rerender(panel("/b"));
    await flush();
    expect(text()).not.toContain("a failed");
    expect(text()).toContain("b one");
  });
});
