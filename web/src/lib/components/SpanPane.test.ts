import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render } from "@testing-library/svelte";
import { ApiError } from "../api";
import type { Span } from "../types";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({ ...(await orig<typeof import("../api")>()), getJSON: (...a: unknown[]) => getJSON(...a) }));
const goto = vi.fn();
vi.mock("$app/navigation", () => ({ goto: (...a: unknown[]) => goto(...a) }));
import RunPage from "./RunPage.svelte";

const API = "/v1/runs/superpipeline/brd_01/run_01";
const BASE = "/runs/superpipeline/brd_01/run_01";
const doc = {
  run: { ref: "superpipeline:brd_01/run_01", state: "failed", card: { title: "A run" } },
  attempts: [{ id: "attempt_a1", session_id: "acps_a1", seq_from: 1, seq_to: 9 }],
  errors: [], cost: {}, verdicts: [],
  sources: { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "ok", errors: "ok", verdicts: "ok" },
};
const spans: Span[] = [
  { trace_id: "t1", span_id: "sp_attempt", name: "attempt", service: "hub", start: "2026-10-04T10:00:00Z", duration_ms: 9000,
    attributes: { "attempt.id": "attempt_a1", "acp.session_id": "acps_a1", "acp.seq_from": "1", "acp.seq_to": "9" } },
  { trace_id: "t1", span_id: "sp_tool", parent_span_id: "sp_attempt", name: "tool_call", service: "hub", start: "2026-10-04T10:00:03Z", duration_ms: 1000,
    attributes: { "attempt.id": "attempt_a1", "acp.seq_from": "3", "acp.seq_to": "4", "otel.status_code": "ERROR" } },
  { trace_id: "t2", span_id: "sp_http", name: "GET /x", service: "api", start: "2026-10-04T10:00:05Z", duration_ms: 10, attributes: {} },
];
const logs = [
  { at: "2026-10-04T10:00:03.5Z", service: "hub", level: "warn", message: "tool log line", trace_id: "t1" },
  { at: "2026-10-04T10:00:08Z", service: "hub", level: "info", message: "later line", trace_id: "t1" },
];
const tool = {
  kind: "tool_call", id: "tc_1", seq_from: 3, seq_to: 4, title: "Write out.md", tool_kind: "edit", status: "failed", redactions: 0,
  input: { path: "out.md" }, output: { content: [{ type: "text", text: { truncated: true, bytes: 40000, head: "denied" } }], raw: null },
};
const items = [{ kind: "prompt", seq: 1, text: "do it with [redacted:anthropic-key]", images: [], redactions: 1 }, tool,
  { kind: "message", seq_from: 9, seq_to: 9, text: "could not write", redactions: 0 }];
const txPage = (its: object[] = items) => ({ attempt_id: "attempt_a1", session_id: "acps_a1", seq_from: 1, seq_to: 9, items: its, next_cursor: null, redactions: 1, truncated_fields: 1 });

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const txCalls = () => getJSON.mock.calls.map(([p]) => String(p)).filter((p) => p.includes("/transcript"));
const pane = () => document.querySelector('aside[aria-label="Span details"]') as HTMLElement | null;
const button = (name: string, within: ParentNode = document) => [...within.querySelectorAll("button")].find((b) => b.textContent?.trim() === name)!;
const click = async (el: Element) => {
  await fireEvent.click(el);
  await flush();
};

let view: ReturnType<typeof render<typeof RunPage>>;
async function mount(query: string, over: { doc?: object; tx?: (path: string) => Promise<unknown>; logs?: () => Promise<unknown> } = {}) {
  getJSON.mockImplementation(async (path: string) => {
    if (path === API) return over.doc ?? doc;
    if (path.startsWith(`${API}/spans`)) return { spans };
    if (path.startsWith(`${API}/logs`)) return over.logs ? over.logs() : { logs, next_cursor: null };
    if (path.startsWith(`${API}/transcript/items/3`))
      return { attempt_id: "attempt_a1", session_id: "acps_a1", item: { ...tool, output: { content: [{ type: "text", text: "denied: the whole text" }], raw: null } } };
    if (path.startsWith(`${API}/transcript`)) return over.tx ? over.tx(path) : txPage();
    if (path.startsWith("/v1/verdicts")) return { verdicts: [] };
    if (path === "/v1/rubrics") return { rubrics: [] };
    return {};
  });
  view = render(RunPage, { board: "brd_01", run: "run_01", query });
  for (let i = 0; i < 4; i++) await flush();
}

beforeEach(() => {
  getJSON.mockReset();
  goto.mockReset();
});
afterEach(cleanup);

describe("the span pane", () => {
  it("fetches no session content until a span is opened, and opening one puts it in the URL", async () => {
    await mount("");
    expect(txCalls()).toEqual([]);
    expect(pane()).toBeNull();
    await click([...document.querySelectorAll("button.span-name")].find((b) => b.textContent?.trim() === "tool_call")!);
    expect(goto.mock.calls[0][0]).toBe(`${BASE}?span=sp_tool`);
    await view.rerender({ board: "brd_01", run: "run_01", query: "span=sp_tool" });
    await flush();
    await flush();
    expect(pane()).not.toBeNull();
    expect(document.querySelector('button.span-name[aria-pressed="true"]')?.textContent?.trim()).toBe("tool_call");
  });

  it("shows the span's details, its log lines, and its request and response", async () => {
    await mount("span=sp_tool");
    const p = pane()!;
    expect(p).not.toBeNull();
    expect(p.querySelector("h2")?.textContent).toBe("tool_call");
    for (const text of ["tool_call", "hub", "1000 ms", "ERROR", "acp.seq_from", "tool log line", "Request", '"path": "out.md"', "Response", "denied… [40000 bytes, cut]", "1 value redacted"]) {
      expect(p.textContent).toContain(text);
    }
    expect(p.textContent).not.toContain("later line");
    expect(txCalls()).toEqual([`${API}/transcript?attempt=attempt_a1&seq_from=3&seq_to=4`]);
    const open = [...p.querySelectorAll("a")].find((a) => a.textContent?.trim() === "Open in transcript ↗")!;
    expect(open.getAttribute("href")).toBe(`${BASE}?tab=transcript&attempt=attempt_a1&seq=3-4`);
  });

  it("names the facts, and says the start in readable time with the ISO in its title", async () => {
    await mount("span=sp_tool");
    const facts = Object.fromEntries([...pane()!.querySelectorAll(".sw-kv > div")].map((d) => [d.querySelector("dt")?.textContent, d.querySelector("dd")]));
    expect(Object.keys(facts)).toEqual(["Service", "Start", "Duration", "Status"]);
    expect(facts.Service?.textContent).toBe("hub");
    expect(facts.Duration?.textContent).toBe("1000 ms");
    expect(facts.Status?.textContent?.trim()).toBe("ERROR");
    const time = facts.Start?.querySelector("time");
    expect(time?.getAttribute("title")).toBe("2026-10-04T10:00:03Z");
    expect(time?.getAttribute("datetime")).toBe("2026-10-04T10:00:03Z");
    expect(time?.textContent).not.toBe("2026-10-04T10:00:03Z");
  });

  it("lists the attributes sorted by key", async () => {
    await mount("span=sp_tool");
    const keys = [...pane()!.querySelectorAll(".sw-attrs th")].map((t) => t.textContent);
    expect(keys).toEqual(["acp.seq_from", "acp.seq_to", "attempt.id", "otel.status_code"]);
    expect(pane()!.querySelector(".sw-attrs tr:last-child td")?.textContent).toBe("ERROR");
  });

  it("says when the span has no log lines, while they load, and when they fail", async () => {
    await mount("span=sp_tool", { logs: async () => ({ logs: [], next_cursor: null }) });
    expect(pane()!.textContent).toContain("No log lines in this span.");
    cleanup();
    await mount("span=sp_tool", { logs: () => new Promise(() => {}) });
    expect(pane()!.textContent).toContain("Loading logs…");
    cleanup();
    await mount("span=sp_tool", { logs: () => Promise.reject(new ApiError(503, "x", "The log source is busy", true)) });
    expect(pane()!.textContent).toContain("The log source is busy");
    expect(getJSON.mock.calls.some(([p]) => String(p) === `${API}/logs?limit=500`)).toBe(true);
  });

  it("says when the span has more steps than the pane shows", async () => {
    await mount("span=sp_tool", { tx: async () => ({ ...txPage(), next_cursor: "c2" }) });
    expect(pane()!.textContent).toContain("More steps than shown here; open the transcript for the rest.");
  });

  it("says nothing of more steps when the span fits the page", async () => {
    await mount("span=sp_tool");
    expect(pane()!.textContent).not.toContain("More steps than shown here");
  });

  it("says Loading session content… while the steps are read", async () => {
    await mount("span=sp_tool", { tx: () => new Promise(() => {}) });
    expect(pane()!.textContent).toContain("Loading session content…");
  });

  it("Show full replaces the cut field with the whole item", async () => {
    await mount("span=sp_tool");
    await click(button("Show full", pane()!));
    await flush();
    expect(txCalls()).toContain(`${API}/transcript/items/3?attempt=attempt_a1&seq_from=3&seq_to=4&full=1`);
    expect(pane()!.textContent).toContain("denied: the whole text");
    expect([...pane()!.querySelectorAll("button")].some((b) => b.textContent?.trim() === "Show full")).toBe(false);
  });

  it("says a span outside the session has no content, and reads none", async () => {
    await mount("span=sp_http");
    expect(pane()!.textContent).toContain("no session content for this span");
    expect(txCalls()).toEqual([]);
  });

  it("says when transcripts are not for this account, and keeps the rest of the pane", async () => {
    await mount("span=sp_tool", { tx: () => Promise.reject(new ApiError(403, "transcripts_forbidden", "no")) });
    expect(pane()!.textContent).toContain("Transcripts aren't available to this account");
    expect(pane()!.textContent).toContain("acp.seq_from");
    expect(document.querySelectorAll(".waterfall li")).toHaveLength(3);
  });

  it("says when the hub is down, and tries again", async () => {
    let down = true;
    await mount("span=sp_tool", { tx: () => (down ? Promise.reject(new ApiError(503, "source_unavailable", "no", true)) : Promise.resolve(txPage())) });
    expect(pane()!.textContent).toContain("Transcript unavailable, try again");
    down = false;
    await click(button("Try again", pane()!));
    await flush();
    expect(pane()!.textContent).toContain('"path": "out.md"');
  });

  it("shows any other refusal as it came", async () => {
    await mount("span=sp_tool", { tx: () => Promise.reject(new ApiError(500, "internal", "the hub broke")) });
    expect(pane()!.textContent).toContain("the hub broke");
  });

  it("says a running attempt is still running", async () => {
    await mount("span=sp_tool", { doc: { ...doc, attempts: [{ ...doc.attempts[0], seq_to: null }] } });
    expect(pane()!.textContent).toContain("Attempt still running; refresh for more");
  });

  it("cites the span, as its checkbox does, and closes", async () => {
    await mount("span=sp_tool");
    await click(button("Cite", pane()!));
    expect(button("Cited", pane()!).getAttribute("aria-pressed")).toBe("true");
    expect((document.querySelector('input[aria-label="Cite tool_call as evidence"]') as HTMLInputElement).checked).toBe(true);
    await click(button("Close", pane()!));
    expect(goto.mock.calls.at(-1)?.[0]).toBe(BASE);
  });

  it("is not modal: no dialog role, and Escape is left alone", async () => {
    await mount("span=sp_tool");
    expect(pane()!.getAttribute("role")).toBeNull();
    expect(pane()!.getAttribute("aria-modal")).toBeNull();
    await fireEvent.keyDown(document, { key: "Escape" });
    expect(goto).not.toHaveBeenCalled();
    expect(pane()).not.toBeNull();
  });
});
