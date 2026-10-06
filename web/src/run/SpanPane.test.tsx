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
import { ApiError } from "../api";
import { RunViewPage } from "./RunViewPage";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const me: Me = { principal: "prn_human01", kind: "human", email: null, via: "session" };
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
const page = (its: object[] = items) => ({ attempt_id: "attempt_a1", session_id: "acps_a1", seq_from: 1, seq_to: 9, items: its, next_cursor: null, redactions: 1, truncated_fields: 1 });

let root: Root;
let host: HTMLElement;
const flush = () => act(async () => {});
const txCalls = () => getJSON.mock.calls.map(([p]) => String(p)).filter((p) => p.includes("/transcript"));
const pane = () => host.querySelector('aside[aria-label="Span details"]') as HTMLElement | null;
const button = (name: string, within: ParentNode = host) => [...within.querySelectorAll("button")].find((b) => b.textContent === name)!;
const click = (el: Element) => act(async () => void el.dispatchEvent(new MouseEvent("click", { bubbles: true })));

async function render(query: string, over: { doc?: object; tx?: (path: string) => Promise<unknown> } = {}) {
  getJSON.mockImplementation(async (path: string) => {
    if (path === API) return over.doc ?? doc;
    if (path.startsWith(`${API}/spans`)) return { spans };
    if (path.startsWith(`${API}/logs`)) return { logs, next_cursor: null };
    if (path.startsWith(`${API}/transcript/items/3`))
      return { attempt_id: "attempt_a1", session_id: "acps_a1", item: { ...tool, output: { content: [{ type: "text", text: "denied: the whole text" }], raw: null } } };
    if (path.startsWith(`${API}/transcript`)) return over.tx ? over.tx(path) : page();
    if (path.startsWith("/v1/verdicts")) return { verdicts: [] };
    if (path === "/v1/rubrics") return { rubrics: [] };
    return {};
  });
  await act(async () => root.render(<RunViewPage board="brd_01" run="run_01" query={query} me={me} />));
  for (let i = 0; i < 4; i++) await flush();
}

beforeEach(() => {
  getJSON.mockReset();
  history.replaceState(null, "", "/");
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

describe("the span pane", () => {
  it("fetches no session content until a span is opened, and opening one puts it in the URL", async () => {
    await render("");
    expect(txCalls()).toEqual([]);
    await click([...host.querySelectorAll("button.span-name")].find((b) => b.textContent === "tool_call")!);
    expect(window.location.pathname + window.location.search).toBe(`${BASE}?span=sp_tool`);
  });

  it("shows the span's details, its log lines, and its request and response", async () => {
    await render("span=sp_tool");
    const p = pane()!;
    expect(p).not.toBeNull();
    for (const text of ["tool_call", "hub", "1000 ms", "ERROR", "acp.seq_from", "tool log line", "Request", '"path": "out.md"', "Response", "denied… [40000 bytes, cut]", "1 value redacted"]) {
      expect(p.textContent).toContain(text);
    }
    expect(p.textContent).not.toContain("later line");
    expect(txCalls()).toEqual([`${API}/transcript?attempt=attempt_a1&seq_from=3&seq_to=4`]);
    const open = [...p.querySelectorAll("a")].find((a) => a.textContent === "Open in transcript ↗")!;
    expect(open.getAttribute("href")).toBe(`${BASE}?tab=transcript&attempt=attempt_a1&seq=3-4`);
  });

  it("says when the span has more steps than the pane shows", async () => {
    await render("span=sp_tool", { tx: async () => ({ ...page(), next_cursor: "c2" }) });
    expect(pane()!.textContent).toContain("More steps than shown here; open the transcript for the rest.");
  });

  it("says nothing of more steps when the span fits the page", async () => {
    await render("span=sp_tool");
    expect(pane()!.textContent).not.toContain("More steps than shown here");
  });

  it("Show full replaces the cut field with the whole item", async () => {
    await render("span=sp_tool");
    await click(button("Show full", pane()!));
    await flush();
    expect(txCalls()).toContain(`${API}/transcript/items/3?attempt=attempt_a1&seq_from=3&seq_to=4&full=1`);
    expect(pane()!.textContent).toContain("denied: the whole text");
    expect([...pane()!.querySelectorAll("button")].some((b) => b.textContent === "Show full")).toBe(false);
  });

  it("says a span outside the session has no content, and reads none", async () => {
    await render("span=sp_http");
    expect(pane()!.textContent).toContain("no session content for this span");
    expect(txCalls()).toEqual([]);
  });

  it("says when transcripts are not for this account, and keeps the rest of the pane", async () => {
    await render("span=sp_tool", { tx: () => Promise.reject(new ApiError(403, "transcripts_forbidden", "no")) });
    expect(pane()!.textContent).toContain("Transcripts aren't available to this account");
    expect(pane()!.textContent).toContain("acp.seq_from");
    expect(host.querySelectorAll(".waterfall li")).toHaveLength(3);
  });

  it("says when the hub is down, and tries again", async () => {
    let down = true;
    await render("span=sp_tool", { tx: () => (down ? Promise.reject(new ApiError(503, "source_unavailable", "no", true)) : Promise.resolve(page())) });
    expect(pane()!.textContent).toContain("Transcript unavailable, try again");
    down = false;
    await click(button("Try again", pane()!));
    await flush();
    expect(pane()!.textContent).toContain('"path": "out.md"');
  });

  it("says a running attempt is still running", async () => {
    await render("span=sp_tool", { doc: { ...doc, attempts: [{ ...doc.attempts[0], seq_to: null }] } });
    expect(pane()!.textContent).toContain("Attempt still running; refresh for more");
  });

  it("cites the span, as its checkbox does, and closes", async () => {
    await render("span=sp_tool");
    await click(button("Cite", pane()!));
    expect(button("Cited", pane()!).getAttribute("aria-pressed")).toBe("true");
    expect((host.querySelector('input[aria-label="Cite tool_call as evidence"]') as HTMLInputElement).checked).toBe(true);
    await click(button("Close", pane()!));
    expect(window.location.pathname + window.location.search).toBe(BASE);
  });
});
