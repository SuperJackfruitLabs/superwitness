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
const a1 = { id: "attempt_a1", session_id: "acps_a1", seq_from: 1, seq_to: 9 };
const doc = {
  run: { ref: "superpipeline:brd_01/run_01", state: "failed", card: { title: "A run" } },
  attempts: [a1], errors: [], cost: {}, verdicts: [],
  sources: { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "ok", errors: "ok", verdicts: "ok" },
};
const spans: Span[] = [
  { trace_id: "t1", span_id: "sp_attempt", name: "attempt", service: "hub", start: "2026-10-04T10:00:00Z", duration_ms: 9000,
    attributes: { "attempt.id": "attempt_a1", "acp.seq_from": "1", "acp.seq_to": "9" } },
  { trace_id: "t1", span_id: "sp_tool", name: "tool_call", service: "hub", start: "2026-10-04T10:00:03Z", duration_ms: 1000,
    attributes: { "attempt.id": "attempt_a1", "acp.seq_from": "3", "acp.seq_to": "4" } },
];
const prompt = { kind: "prompt", seq: 1, text: "do it with [redacted:anthropic-key]", images: [], redactions: 1 };
const reasoning = { kind: "reasoning", seq_from: 2, seq_to: 2, text: "think first", redactions: 0 };
const tool = { kind: "tool_call", id: "tc_1", seq_from: 3, seq_to: 4, title: "Write out.md", tool_kind: "edit", status: "failed", redactions: 0,
  input: { path: "out.md" }, output: { content: [{ type: "text", text: "denied" }], raw: null } };
const message = { kind: "message", seq_from: 9, seq_to: 9, text: "could not write", redactions: 0 };
const page = (items: object[], next: string | null = null) =>
  ({ attempt_id: "attempt_a1", session_id: "acps_a1", seq_from: 1, seq_to: 9, items, next_cursor: next, redactions: 1, truncated_fields: 0 });

let root: Root;
let host: HTMLElement;
const flush = () => act(async () => {});
const calls = () => getJSON.mock.calls.map(([p]) => String(p));
const cards = () => [...host.querySelectorAll<HTMLLIElement>(".transcript > li")];
const card = (kind: string) => host.querySelector<HTMLLIElement>(`.transcript > li[data-kind="${kind}"]`)!;
const button = (name: string, within: ParentNode = host) => [...within.querySelectorAll("button")].find((b) => b.textContent === name)!;
const click = (el: Element) => act(async () => void el.dispatchEvent(new MouseEvent("click", { bubbles: true })));

async function render(query: string, over: { doc?: object; tx?: (path: string) => Promise<unknown> } = {}) {
  getJSON.mockImplementation(async (path: string) => {
    if (path === API) return over.doc ?? doc;
    if (path.startsWith(`${API}/spans`)) return { spans };
    if (path.startsWith(`${API}/transcript`)) return over.tx ? over.tx(path) : page([prompt, reasoning, tool, message]);
    if (path.startsWith("/v1/verdicts")) return { verdicts: [] };
    if (path === "/v1/rubrics") return { rubrics: [] };
    return {};
  });
  await act(async () => root.render(<RunViewPage board="brd_01" run="run_01" query={query} me={me} />));
  for (let i = 0; i < 6; i++) await flush();
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

describe("the Transcript tab", () => {
  it("sits between Trace and Logs", async () => {
    await render("tab=transcript");
    expect([...host.querySelectorAll('[role="tab"]')].map((t) => t.textContent).slice(0, 3)).toEqual(["Trace", "Transcript", "Logs"]);
    expect(host.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toBe("Transcript");
  });

  it("shows the attempt as a conversation", async () => {
    await render("tab=transcript");
    expect(calls()).toContain(`${API}/transcript?attempt=attempt_a1`);
    expect(cards().map((c) => c.dataset.kind)).toEqual(["prompt", "reasoning", "tool_call", "message"]);
    expect(card("prompt").textContent).toContain("[redacted:anthropic-key]");
    expect(card("prompt").textContent).toContain("1 redacted");
    expect(card("reasoning").querySelector("details")!.open).toBe(false);
    expect(card("tool_call").className).toBe("failed");
    expect(card("tool_call").querySelector("details")!.open).toBe(true);
    expect(card("tool_call").textContent).toContain('"path": "out.md"');
    expect(card("message").textContent).toContain("could not write");
  });

  it("links each card to the narrowest span that holds it", async () => {
    await render("tab=transcript");
    const link = (kind: string) => [...card(kind).querySelectorAll("a")].find((a) => a.textContent === "span ↗")!.getAttribute("href");
    expect(link("tool_call")).toBe(`${BASE}?span=sp_tool`);
    expect(link("prompt")).toBe(`${BASE}?span=sp_attempt`);
  });

  it("opens at the seq in the URL", async () => {
    await render("tab=transcript&attempt=attempt_a1&seq=3-4");
    const current = host.querySelectorAll('.transcript > li[aria-current="true"]');
    expect(current).toHaveLength(1);
    expect((current[0] as HTMLElement).dataset.kind).toBe("tool_call");
  });

  it("loads pages until the linked seq is on screen", async () => {
    await render("tab=transcript&attempt=attempt_a1&seq=3-4", {
      tx: async (path) => (path.includes("cursor=c2") ? page([tool, message]) : page([prompt], "c2")),
    });
    expect(calls()).toContain(`${API}/transcript?attempt=attempt_a1&cursor=c2`);
    expect((host.querySelector('.transcript > li[aria-current="true"]') as HTMLElement).dataset.kind).toBe("tool_call");
  });

  it("loads more on request", async () => {
    await render("tab=transcript", { tx: async (path) => (path.includes("cursor=c2") ? page([tool, message]) : page([prompt, reasoning], "c2")) });
    expect(cards()).toHaveLength(2);
    await click(button("Load more"));
    await flush();
    expect(cards()).toHaveLength(4);
    expect([...host.querySelectorAll("button")].some((b) => b.textContent === "Load more")).toBe(false);
  });

  it("cites a card as a session range in the verdict drawer", async () => {
    await render("tab=transcript");
    await click(button("Cite", card("tool_call")));
    expect(button("Cited", card("tool_call")).getAttribute("aria-pressed")).toBe("true");
    await click(button("Record verdict"));
    await flush();
    expect(host.querySelector(".drawer")!.textContent).toContain("1 cited: 1 step from the transcript");
  });

  it("offers a picker when the run has several attempts, and always names the attempt", async () => {
    const a2 = { id: "attempt_a2", session_id: "acps_a2", seq_from: 10, seq_to: 12 };
    await render("tab=transcript", { doc: { ...doc, attempts: [a1, a2] } });
    const select = host.querySelector<HTMLSelectElement>("label.picker select")!;
    expect(select.value).toBe("attempt_a1");
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(select, "attempt_a2");
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(window.location.search).toBe("?tab=transcript&attempt=attempt_a2");
    expect(calls().filter((p) => p.includes("/transcript") && !p.includes("attempt="))).toEqual([]);
  });

  // One render per state: a run view keeps its reads while it stays mounted.
  it("says when transcripts are not for this account, and the rest of the page still works", async () => {
    await render("tab=transcript", { tx: () => Promise.reject(new ApiError(403, "transcripts_forbidden", "no")) });
    expect(host.textContent).toContain("Transcripts aren't available to this account");
    expect(host.textContent).toContain("A run");
    expect(button("Record verdict")).toBeDefined();
  });

  it("says when the hub is down", async () => {
    await render("tab=transcript", { tx: () => Promise.reject(new ApiError(503, "source_unavailable", "no", true)) });
    expect(host.textContent).toContain("Transcript unavailable, try again");
  });

  it("says when the attempt is still running", async () => {
    await render("tab=transcript", { doc: { ...doc, attempts: [{ ...a1, seq_to: null }] } });
    expect(host.textContent).toContain("Attempt still running; refresh for more");
  });

  it("asks for the whole field in the attempt's range, and shows it", async () => {
    const cut = { ...tool, input: { truncated: true, bytes: 20000, head: "the start of a long input" } };
    const whole = { ...tool, input: { path: "the whole long input" } };
    await render("tab=transcript", {
      tx: async (path) => (path.includes("/transcript/items/") ? { item: whole } : page([prompt, cut])),
    });
    expect(card("tool_call").textContent).toContain("the start of a long input");
    await click(button("Show full", card("tool_call")));
    await flush();
    expect(calls()).toContain(`${API}/transcript/items/3?attempt=attempt_a1&seq_from=1&seq_to=9&full=1`);
    expect(card("tool_call").textContent).toContain("the whole long input");
  });

  it("shows the question a permission asked", async () => {
    const permission = { kind: "permission", seq_from: 5, seq_to: 6, title: "Allow writing out.md?", options: [{ name: "Allow", optionId: "allow" }, { name: "Deny", optionId: "deny" }], outcome: "allow", redactions: 0 };
    await render("tab=transcript", { tx: async () => page([prompt, permission]) });
    expect(card("permission").textContent).toContain("Allow writing out.md?");
    expect(card("permission").textContent).toContain("Asked to choose: Allow / Deny");
  });
});
