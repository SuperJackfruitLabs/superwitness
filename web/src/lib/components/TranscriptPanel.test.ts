import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render } from "@testing-library/svelte";
import { ApiError } from "../api";
import { createEvidence, MAX_EVIDENCE } from "../evidence.svelte";
import { rangeRef } from "../run/transcript";
import type { Span } from "../types";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({ ...(await orig<typeof import("../api")>()), getJSON: (...a: unknown[]) => getJSON(...a) }));
const goto = vi.fn();
vi.mock("$app/navigation", () => ({ goto: (...a: unknown[]) => goto(...a) }));
import RunPage from "./RunPage.svelte";
import TranscriptPanel from "./TranscriptPanel.svelte";

// A port of TranscriptPanel.test.tsx, every case, plus the states the plan names for Part 9.
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

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const calls = () => getJSON.mock.calls.map(([p]) => String(p));
const cards = () => [...document.querySelectorAll<HTMLLIElement>(".transcript > li")];
const card = (kind: string) => document.querySelector<HTMLLIElement>(`.transcript > li[data-kind="${kind}"]`)!;
const button = (name: string, within: ParentNode = document) => [...within.querySelectorAll("button")].find((b) => b.textContent?.trim() === name)!;
const text = () => document.body.textContent ?? "";
const click = async (el: Element) => {
  await fireEvent.click(el);
  await flush();
};

async function mount(query: string, over: { doc?: object; tx?: (path: string) => Promise<unknown> } = {}) {
  getJSON.mockImplementation(async (path: string) => {
    if (path === API) return over.doc ?? doc;
    if (path.startsWith(`${API}/spans`)) return { spans };
    if (path.startsWith(`${API}/transcript`)) return over.tx ? over.tx(path) : page([prompt, reasoning, tool, message]);
    if (path.startsWith("/v1/verdicts")) return { verdicts: [] };
    if (path === "/v1/rubrics") return { rubrics: [] };
    return {};
  });
  render(RunPage, { board: "brd_01", run: "run_01", query });
  for (let i = 0; i < 6; i++) await flush();
}

let scrolled: Element[] = [];
beforeEach(() => {
  getJSON.mockReset();
  goto.mockReset();
  scrolled = [];
  Element.prototype.scrollIntoView = function (this: Element) {
    scrolled.push(this);
  };
});
afterEach(cleanup);

describe("the Transcript tab", () => {
  it("cites a card as a session range in the verdict drawer", async () => {
    await mount("tab=transcript");
    await click(button("Cite", card("tool_call")));
    expect(button("Cited", card("tool_call")).getAttribute("aria-pressed")).toBe("true");
    await click(button("Record verdict"));
    await flush();
    expect(document.querySelector(".drawer")!.textContent).toContain("1 cited: 1 step from the transcript");
  });

  it("sits between Trace and Logs", async () => {
    await mount("tab=transcript");
    expect([...document.querySelectorAll('[role="tab"]')].map((t) => t.textContent?.trim()).slice(0, 3)).toEqual(["Trace", "Transcript", "Logs"]);
    expect(document.querySelector('[role="tab"][aria-selected="true"]')?.textContent?.trim()).toBe("Transcript");
  });

  it("shows the attempt as a conversation", async () => {
    await mount("tab=transcript");
    expect(calls()).toContain(`${API}/transcript?attempt=attempt_a1`);
    expect(cards().map((c) => c.dataset.kind)).toEqual(["prompt", "reasoning", "tool_call", "message"]);
    expect(card("prompt").textContent).toContain("[redacted:anthropic-key]");
    expect(card("prompt").textContent).toContain("1 redacted");
    expect(card("reasoning").querySelector("details")!.open).toBe(false);
    expect(card("tool_call").classList.contains("failed")).toBe(true);
    expect(card("tool_call").querySelector("details")!.open).toBe(true);
    expect(card("tool_call").querySelector("summary")!.textContent).toContain("Write out.md · edit · failed");
    expect(card("tool_call").textContent).toContain('"path": "out.md"');
    expect(card("message").textContent).toContain("could not write");
  });

  it("draws a redaction marker as a mark inside the text", async () => {
    await mount("tab=transcript");
    const marks = [...card("prompt").querySelectorAll("mark.sw-redaction")];
    expect(marks.map((m) => m.textContent)).toEqual(["[redacted:anthropic-key]"]);
    expect(card("prompt").querySelector("pre")!.textContent).toBe("do it with [redacted:anthropic-key]");
  });

  it("marks a failed tool call with a danger marker and the word", async () => {
    await mount("tab=transcript");
    expect(card("tool_call").querySelector(".sw-step__fail")?.textContent?.trim()).toBe("failed");
  });

  it("links each card to the narrowest span that holds it", async () => {
    await mount("tab=transcript");
    const link = (kind: string) => [...card(kind).querySelectorAll("a")].find((a) => a.textContent?.trim() === "span ↗")!.getAttribute("href");
    expect(link("tool_call")).toBe(`${BASE}?span=sp_tool`);
    expect(link("prompt")).toBe(`${BASE}?span=sp_attempt`);
  });

  it("opens at the seq in the URL, outlined and scrolled to the centre", async () => {
    await mount("tab=transcript&attempt=attempt_a1&seq=3-4");
    const current = document.querySelectorAll('.transcript > li[aria-current="true"]');
    expect(current).toHaveLength(1);
    expect((current[0] as HTMLElement).dataset.kind).toBe("tool_call");
    expect(scrolled).toContain(current[0]);
  });

  it("opens at a single seq too", async () => {
    await mount("tab=transcript&seq=9");
    expect((document.querySelector('.transcript > li[aria-current="true"]') as HTMLElement).dataset.kind).toBe("message");
  });

  it("loads pages until the linked seq is on screen", async () => {
    await mount("tab=transcript&attempt=attempt_a1&seq=3-4", {
      tx: async (path) => (path.includes("cursor=c2") ? page([tool, message]) : page([prompt], "c2")),
    });
    expect(calls()).toContain(`${API}/transcript?attempt=attempt_a1&cursor=c2`);
    expect((document.querySelector('.transcript > li[aria-current="true"]') as HTMLElement).dataset.kind).toBe("tool_call");
  });

  it("chases a seq on page 3, and gives up after 20 pages", async () => {
    await mount("tab=transcript&seq=3-4", {
      tx: async (path) => {
        const n = Number(/cursor=p(\d+)/.exec(path)?.[1] ?? 1);
        return n === 3 ? page([tool], "p4") : page([{ ...message, seq_from: 100 + n, seq_to: 100 + n }], `p${n + 1}`);
      },
    });
    expect((document.querySelector('.transcript > li[aria-current="true"]') as HTMLElement).dataset.kind).toBe("tool_call");
    cleanup();
    getJSON.mockReset();
    await mount("tab=transcript&seq=3-4", {
      tx: async (path) => {
        const n = Number(/cursor=p(\d+)/.exec(path)?.[1] ?? 1);
        return page([{ ...message, seq_from: 100 + n, seq_to: 100 + n }], `p${n + 1}`);
      },
    });
    for (let i = 0; i < 30; i++) await flush();
    expect(calls().filter((p) => p.includes("/transcript?") && p.includes("cursor="))).toHaveLength(20);
  });

  it("loads more on request", async () => {
    await mount("tab=transcript", { tx: async (path) => (path.includes("cursor=c2") ? page([tool, message]) : page([prompt, reasoning], "c2")) });
    expect(cards()).toHaveLength(2);
    await click(button("Load more"));
    await flush();
    expect(cards()).toHaveLength(4);
    expect(button("Load more")).toBeUndefined();
  });

  it("keeps the cards when loading more fails, and offers to try again", async () => {
    await mount("tab=transcript", {
      tx: async (path) => {
        if (path.includes("cursor=c2")) throw new ApiError(0, "network", "superwitness could not be reached");
        return page([prompt], "c2");
      },
    });
    await click(button("Load more"));
    expect(cards()).toHaveLength(1);
    expect(text()).toContain("superwitness could not be reached");
    expect(button("Try loading more again")).toBeDefined();
  });

  it("cites a card as a session range", async () => {
    await mount("tab=transcript");
    await click(button("Cite", card("tool_call")));
    expect(button("Cited", card("tool_call")).getAttribute("aria-pressed")).toBe("true");
    await click(button("Cited", card("tool_call")));
    expect(button("Cite", card("tool_call")).getAttribute("aria-pressed")).toBe("false");
  });

  it("counts cited cards toward the 100 cap", async () => {
    getJSON.mockImplementation(async (path: string) => (path.startsWith(`${API}/transcript`) ? page([prompt, tool]) : { spans: [] }));
    const evidence = createEvidence();
    for (let i = 0; i < MAX_EVIDENCE; i++) evidence.toggle(`s${i}`);
    render(TranscriptPanel, { api: API, page: BASE, doc, attempt: null, seq: null, evidence });
    for (let i = 0; i < 4; i++) await flush();
    expect(button("Cite", card("tool_call")).disabled).toBe(true);
    evidence.toggle("s0");
    await flush();
    await click(button("Cite", card("tool_call")));
    expect(evidence.has(rangeRef("acps_a1", 3, 4))).toBe(true);
    expect(button("Cite", card("prompt")).disabled).toBe(true);
  });

  it("offers a picker when the run has several attempts, and always names the attempt", async () => {
    const a2 = { id: "attempt_a2", session_id: "acps_a2", seq_from: 10, seq_to: 12 };
    await mount("tab=transcript", { doc: { ...doc, attempts: [a1, a2] } });
    const select = document.querySelector<HTMLSelectElement>("label.picker select")!;
    expect(select.value).toBe("attempt_a1");
    await fireEvent.change(select, { target: { value: "attempt_a2" } });
    expect(goto.mock.calls[0][0]).toBe(`${BASE}?tab=transcript&attempt=attempt_a2`);
    expect(calls().filter((p) => p.includes("/transcript") && !p.includes("attempt="))).toEqual([]);
  });

  it("reads the attempt named in the URL", async () => {
    const a2 = { id: "attempt_a2", session_id: "acps_a2", seq_from: 10, seq_to: 12 };
    await mount("tab=transcript&attempt=attempt_a2", { doc: { ...doc, attempts: [a1, a2] } });
    expect(calls()).toContain(`${API}/transcript?attempt=attempt_a2`);
    expect(document.querySelector<HTMLSelectElement>("label.picker select")!.value).toBe("attempt_a2");
  });

  it("says when the URL names an attempt the run does not have", async () => {
    const a2 = { id: "attempt_a2", session_id: "acps_a2", seq_from: 10, seq_to: 12 };
    await mount("tab=transcript&attempt=attempt_zz", { doc: { ...doc, attempts: [a1, a2] } });
    expect(text()).toContain("This run has no attempt attempt_zz.");
    expect([...document.querySelectorAll("option")].map((o) => o.textContent)).toContain("Choose an attempt…");
  });

  it("says when the run has no attempts", async () => {
    await mount("tab=transcript", { doc: { ...doc, attempts: [] } });
    expect(text()).toContain("No attempts.");
  });

  it("says when the attempt has no session content", async () => {
    await mount("tab=transcript", { tx: async () => page([]) });
    expect(text()).toContain("No session content in this attempt.");
  });

  it("says it is loading", async () => {
    await mount("tab=transcript", { tx: () => new Promise(() => {}) });
    expect(text()).toContain("Loading the transcript…");
  });

  // One render per state: a run view keeps its reads while it stays mounted.
  it("says when transcripts are not for this account, and the rest of the page still works", async () => {
    await mount("tab=transcript", { tx: () => Promise.reject(new ApiError(403, "transcripts_forbidden", "no")) });
    expect(text()).toContain("Transcripts aren't available to this account");
    expect(text()).toContain("A run");
    expect(button("Record verdict")).toBeDefined();
  });

  it("says when the hub is down, and tries again", async () => {
    let down = true;
    await mount("tab=transcript", {
      tx: () => (down ? Promise.reject(new ApiError(503, "source_unavailable", "no", true)) : Promise.resolve(page([message]))),
    });
    expect(text()).toContain("Transcript unavailable, try again");
    down = false;
    await click(button("Try again"));
    await flush();
    expect(cards()).toHaveLength(1);
  });

  it("shows any other refusal as it came", async () => {
    await mount("tab=transcript", { tx: () => Promise.reject(new ApiError(500, "internal", "Something broke")) });
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Something broke");
  });

  it("says when the attempt is still running", async () => {
    await mount("tab=transcript", { doc: { ...doc, attempts: [{ ...a1, seq_to: null }] } });
    expect(text()).toContain("Attempt still running; refresh for more");
  });

  it("asks for the whole field in the attempt's range, and shows it", async () => {
    const cut = { ...tool, input: { truncated: true, bytes: 20000, head: "the start of a long input" } };
    const whole = { ...tool, input: { path: "the whole long input" } };
    await mount("tab=transcript", {
      tx: async (path) => (path.includes("/transcript/items/") ? { item: whole } : page([prompt, cut])),
    });
    expect(card("tool_call").textContent).toContain("the start of a long input");
    await click(button("Show full", card("tool_call")));
    await flush();
    expect(calls()).toContain(`${API}/transcript/items/3?attempt=attempt_a1&seq_from=1&seq_to=9&full=1`);
    expect(card("tool_call").textContent).toContain("the whole long input");
  });

  it("says when the whole field could not be read", async () => {
    const cut = { ...message, text: { truncated: true, bytes: 20000, head: "a long message" } };
    await mount("tab=transcript", {
      tx: async (path) => {
        if (path.includes("/transcript/items/")) throw new ApiError(500, "internal", "Could not read the item");
        return page([cut]);
      },
    });
    await click(button("Show full", card("message")));
    await flush();
    expect(text()).toContain("Could not read the item");
  });

  it("shows the question a permission asked", async () => {
    const permission = { kind: "permission", seq_from: 5, seq_to: 6, title: "Allow writing out.md?", options: [{ name: "Allow", optionId: "allow" }, { name: "Deny", optionId: "deny" }], outcome: "allow", redactions: 0 };
    await mount("tab=transcript", { tx: async () => page([prompt, permission]) });
    expect(card("permission").textContent).toContain("Allow writing out.md?");
    expect(card("permission").textContent).toContain("Asked to choose: Allow / Deny");
    expect(card("permission").textContent).toContain("Outcome: allow");
  });

  it("says a permission is pending when it has no outcome, and when it offered no options", async () => {
    const permission = { kind: "permission", seq: 5, options: [], redactions: 0 };
    await mount("tab=transcript", { tx: async () => page([permission]) });
    expect(card("permission").textContent).toContain("Asked to choose: no options");
    expect(card("permission").textContent).toContain("Outcome: pending");
  });

  it("draws every other kind of step", async () => {
    const items = [
      { ...prompt, images: [{ name: "a.png", mimeType: "image/png" }, { name: "b.png", mimeType: "image/png" }] },
      { kind: "state", seq: 2, status: "cancelled", reason: "stopped by hand" },
      { kind: "error", seq: 3, error_kind: "harness", message: "it fell over" },
      { kind: "other", seq: 4, type: "usage_update" },
      { kind: "message", seq: 5, seq_to: 6, partial: true, text: "late" },
    ];
    await mount("tab=transcript", { tx: async () => page(items) });
    expect(card("prompt").textContent).toContain("Images: a.png, b.png");
    expect(card("state").textContent).toContain("cancelled · stopped by hand");
    expect(card("error").textContent).toContain("harness: it fell over");
    expect(card("other").textContent).toContain("An event of type usage_update, shown by type only.");
    expect(card("other").textContent).toContain("Event");
    expect(card("message").textContent).toContain("started earlier");
    expect(card("message").textContent).toContain("#5–6");
    expect(card("state").textContent).toContain("#2");
  });

  it("wraps a 5,000-character line instead of widening the page", async () => {
    await mount("tab=transcript", { tx: async () => page([{ ...message, text: "x".repeat(5000) }]) });
    const pre = card("message").querySelector("pre")!;
    expect(pre.classList.contains("sw-value")).toBe(true);
  });
});
