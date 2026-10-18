import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render } from "@testing-library/svelte";
import { ApiError } from "../api";
import type { Me } from "../types";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({ ...(await orig<typeof import("../api")>()), getJSON: (...a: unknown[]) => getJSON(...a) }));
const goto = vi.fn();
vi.mock("$app/navigation", () => ({ goto: (...a: unknown[]) => goto(...a) }));
import LogsPanel from "./LogsPanel.svelte";
import RunPage from "./RunPage.svelte";

// The logs, errors, attempts and verdict cases of RunViewPage.test.tsx, plus the states Part 10 names.
const me: Me = { principal: "prn_human01", kind: "human", email: null, via: "session" };
const API = "/v1/runs/superpipeline/brd_01/run_01";
const BASE = "/runs/superpipeline/brd_01/run_01";
const DIGEST = "sha256:c1a88701c4409ae0eafb5f51ec7dabbe5f18037c1256baa045ab8cabf7ed3426";
const doc = {
  run: { ref: "superpipeline:brd_01/run_01", agent: "prn_agent01", state: "running", started_at: "2026-10-04T10:00:00Z", ended_at: null, card: { title: "A run" } },
  attempts: [{ id: "attempt_a1", session_id: "acps_a1", seq_from: 1, seq_to: 9, state: "completed", fingerprint: { digest: DIGEST, harness: "hermes", harness_version: "0.4.2", model: "claude-sonnet-5-5" } }],
  errors: [{ at: "2026-10-04T10:03:02Z", service: "agentpod-hub", message: "bridge heartbeat failed: 502 from superpipeline" }],
  cost: { status: "unreported" },
  verdicts: [{ id: "gate_1", kind: "gate", standard: "gate:review", status: "approved", value: { decision: "pass" }, judge: "prn_human02" }],
  sources: { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "ok", errors: "ok", verdicts: "ok" },
};
const hv = (id: string, over: object) => ({
  id, kind: "review", subject_kind: "run", subject_ref: "superpipeline:brd_01/run_01", judge: "prn_human01", judge_kind: "human",
  standard: "rubric:release@1", value: { decision: "pass" }, comment: "", evidence_refs: [], supersedes: null, superseded_by: null, created_at: "2026-10-06T10:00:00Z", ...over,
});
const deferred = <T>() => {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((a, b) => ((resolve = a), (reject = b)));
  return { promise, resolve, reject };
};
const line = (m: string, level = "info") => ({ at: "2026-10-04T10:02:03Z", service: "s", level, message: m });

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const text = () => document.body.textContent ?? "";
const calls = () => getJSON.mock.calls.map(([p]) => String(p));
const button = (name: string) => [...document.querySelectorAll("button")].find((b) => b.textContent?.trim() === name);
const click = async (el: Element) => {
  await fireEvent.click(el);
  await flush();
};
const page = async (query: string, impl: (path: string) => Promise<unknown>, who: Me = me) => {
  getJSON.mockImplementation(impl);
  render(RunPage, { board: "brd_01", run: "run_01", query, me: who });
  for (let i = 0; i < 4; i++) await flush();
};
const standard = (d: object = doc) => async (path: string) => {
  if (path === API) return d;
  if (path.startsWith(`${API}/logs`)) return { logs: [line("attempt opened"), line("bridge heartbeat failed", "error")], next_cursor: null };
  if (path.startsWith("/v1/verdicts?subject_kind=run")) return { verdicts: [hv("vrd_1", { superseded_by: "vrd_2", value: { decision: "fail" } }), hv("vrd_2", { supersedes: "vrd_1", comment: "Clear and complete." })] };
  if (path.startsWith("/v1/verdicts?subject_kind=attempt")) return { verdicts: [hv("vrd_3", { subject_kind: "attempt", subject_ref: "attempt_a1", judge: "prn_human02", created_at: "2026-10-05T10:00:00Z" })] };
  return {};
};

beforeEach(() => {
  getJSON.mockReset();
  goto.mockReset();
});
afterEach(cleanup);

describe("the Logs tab", () => {
  it("reads 100 lines a page into a table with a header row and readable times", async () => {
    await page("tab=logs", standard());
    expect(calls()).toContain(`${API}/logs?limit=100`);
    expect([...document.querySelectorAll(".sw-table thead th")].map((t) => t.textContent?.trim())).toEqual(["Time", "Level", "Service", "Message"]);
    const rows = document.querySelectorAll(".sw-table tbody tr");
    expect(rows).toHaveLength(2);
    const time = rows[0].querySelector("time")!;
    expect(time.getAttribute("title")).toBe("2026-10-04T10:02:03Z");
    expect(time.textContent).not.toBe("2026-10-04T10:02:03Z");
    expect(rows[1].textContent).toContain("bridge heartbeat failed");
    expect(rows[1].classList.contains("err")).toBe(true);
  });

  it("filters by level on the server", async () => {
    await page("tab=logs", standard());
    const sel = document.querySelector<HTMLSelectElement>("label.sw-level select")!;
    expect([...sel.options].map((o) => o.textContent)).toEqual(["all", "debug", "info", "warn", "error"]);
    await fireEvent.change(sel, { target: { value: "error" } });
    await flush();
    expect(calls()).toContain(`${API}/logs?limit=100&level=error`);
  });

  it("says when there are no lines", async () => {
    await page("tab=logs", async (path) => (path === API ? doc : { logs: [], next_cursor: null }));
    expect(text()).toContain("No log lines.");
  });

  it("says it is loading", async () => {
    await page("tab=logs", (path) => (path === API ? Promise.resolve(doc) : new Promise(() => {})));
    expect(text()).toContain("Loading logs…");
  });

  it("shows a read failure with Try again, and recovers", async () => {
    let fail = true;
    await page("tab=logs", async (path) => {
      if (path === API) return doc;
      if (fail) throw new ApiError(0, "network", "superwitness could not be reached");
      return { logs: [line("back")], next_cursor: null };
    });
    expect(text()).toContain("superwitness could not be reached");
    expect(text()).not.toContain("No log lines.");
    fail = false;
    await click(button("Try again")!);
    expect(text()).toContain("back");
  });

  it("loads more, and keeps the lines when loading more fails", async () => {
    let fail = false;
    await page("tab=logs", async (path) => {
      if (path === API) return doc;
      if (path.includes("cursor=c2")) {
        if (fail) throw new ApiError(0, "network", "superwitness could not be reached");
        return { logs: [line("second")], next_cursor: null };
      }
      return { logs: [line("first")], next_cursor: "c2" };
    });
    fail = true;
    await click(button("Load more")!);
    expect(text()).toContain("first");
    expect(text()).toContain("superwitness could not be reached");
    fail = false;
    await click(button("Try loading more again")!);
    expect(calls()).toContain(`${API}/logs?limit=100&cursor=c2`);
    expect(text()).toContain("second");
    expect(button("Load more")).toBeUndefined();
  });

  it("drops a log response that arrives after the level changed", async () => {
    const slow = deferred<{ logs: object[]; next_cursor: null }>();
    await page("tab=logs", (path) => {
      if (path === API) return Promise.resolve(doc);
      if (path.includes("level=error")) return Promise.resolve({ logs: [line("an error line")], next_cursor: null });
      return slow.promise;
    });
    await fireEvent.change(document.querySelector("select")!, { target: { value: "error" } });
    await flush();
    expect(text()).toContain("an error line");
    slow.resolve({ logs: [line("an unfiltered line")], next_cursor: null });
    await flush();
    expect(text()).not.toContain("an unfiltered line");
    expect(text()).toContain("an error line");
  });

  it("drops a log load-more that was in flight when the level changed", async () => {
    const more = deferred<{ logs: object[]; next_cursor: null }>();
    await page("tab=logs", (path) => {
      if (path === API) return Promise.resolve(doc);
      if (path.includes("cursor=")) return more.promise;
      if (path.includes("level=error")) return Promise.resolve({ logs: [line("an error line")], next_cursor: null });
      return Promise.resolve({ logs: [line("first")], next_cursor: "c2" });
    });
    await click(button("Load more")!);
    await fireEvent.change(document.querySelector("select")!, { target: { value: "error" } });
    await flush();
    more.resolve({ logs: [line("late log")], next_cursor: null });
    await flush();
    expect(text()).not.toContain("late log");
    expect(text()).toContain("an error line");
  });

  it("LogsPanel drops a response from the previous base", async () => {
    const slow = deferred<{ logs: object[]; next_cursor: null }>();
    getJSON.mockImplementation((path: string) => (path.startsWith("/a/") ? slow.promise : Promise.resolve({ logs: [line("b line")], next_cursor: null })));
    const view = render(LogsPanel, { api: "/a" });
    await view.rerender({ api: "/b" });
    await flush();
    slow.resolve({ logs: [line("a line")], next_cursor: null });
    await flush();
    expect(text()).not.toContain("a line");
    expect(text()).toContain("b line");
  });
});

describe("the Errors tab", () => {
  it("lists the run's errors with readable times", async () => {
    await page("tab=errors", standard());
    expect([...document.querySelectorAll(".sw-table thead th")].map((t) => t.textContent?.trim())).toEqual(["Time", "Service", "Message"]);
    const row = document.querySelector(".sw-table tbody tr")!;
    expect(row.textContent).toContain("agentpod-hub");
    expect(row.textContent).toContain("bridge heartbeat failed: 502 from superpipeline");
    expect(row.querySelector("time")!.getAttribute("title")).toBe("2026-10-04T10:03:02Z");
  });

  it("says when there are none, and missing values are unknown", async () => {
    await page("tab=errors", standard({ ...doc, errors: [] }));
    expect(text()).toContain("No errors.");
    cleanup();
    await page("tab=errors", standard({ ...doc, errors: [{ message: "odd" }] }));
    expect(document.querySelector(".sw-table tbody tr")!.textContent).toContain("unknown");
  });
});

describe("the Attempts tab", () => {
  it("opens the tab named in the URL", async () => {
    await page("tab=attempts", standard());
    expect(document.querySelector('[role="tab"][aria-selected="true"]')?.textContent?.trim()).toBe("Attempts");
    expect(text()).toContain("attempt_a1");
  });

  it("draws a table with the fingerprint, the digest wrapping inside its cell", async () => {
    await page("tab=attempts", standard());
    expect([...document.querySelectorAll(".sw-table thead th")].map((t) => t.textContent?.trim())).toEqual(["Attempt", "State", "Configuration fingerprint", "Harness", "Model"]);
    const cells = [...document.querySelectorAll(".sw-table tbody td")].map((c) => c.textContent?.trim());
    expect(cells).toEqual(["attempt_a1", "completed", DIGEST, "hermes 0.4.2", "claude-sonnet-5-5"]);
    expect(document.querySelector(".sw-digest")?.textContent).toBe(DIGEST);
  });

  it("says unknown for missing values, and No attempts. for none", async () => {
    await page("tab=attempts", standard({ ...doc, attempts: [{ id: "attempt_a1" }] }));
    const cells = [...document.querySelectorAll(".sw-table tbody td")].map((c) => c.textContent?.trim());
    expect(cells).toEqual(["attempt_a1", "unknown", "unknown", "unknown unknown", "unknown"]);
    cleanup();
    await page("tab=attempts", standard({ ...doc, attempts: [] }));
    expect(text()).toContain("No attempts.");
  });
});

describe("the Verdicts tab", () => {
  it("reads verdicts for the run and for each attempt, beside the gate decisions", async () => {
    await page("tab=verdicts", standard());
    expect(calls()).toContain("/v1/verdicts?subject_kind=run&subject_ref=superpipeline%3Abrd_01%2Frun_01");
    expect(calls()).toContain("/v1/verdicts?subject_kind=attempt&subject_ref=attempt_a1");
    expect(text()).toContain("gate:review");
    expect(document.querySelector("#vrd_1")?.className).toContain("superseded");
    expect(document.querySelector("#vrd_3")).not.toBeNull();
    // Revise: only on the viewer's own, unsuperseded verdict (vrd_2); not vrd_1 (superseded), not vrd_3 (someone else's).
    const revise = [...document.querySelectorAll("button")].filter((b) => b.textContent?.trim() === "Revise");
    expect(revise).toHaveLength(1);
    expect(revise[0].closest("li")?.id).toBe("vrd_2");
  });

  it("lists recorded verdicts oldest first, with judge, time, attempt and comment", async () => {
    await page("tab=verdicts", standard());
    expect([...document.querySelectorAll("li.verdict[id^=vrd]")].map((l) => l.id)).toEqual(["vrd_3", "vrd_1", "vrd_2"]);
    const v2 = document.querySelector("#vrd_2")!;
    expect(v2.textContent).toContain("pass");
    expect(v2.textContent).toContain("rubric:release@1");
    expect(v2.textContent).toContain("prn_human01 (human)");
    expect(v2.querySelector("time")!.getAttribute("title")).toBe("2026-10-06T10:00:00Z");
    expect(v2.textContent).toContain("Clear and complete.");
    expect(document.querySelector("#vrd_3")!.textContent).toContain("attempt_a1");
  });

  it("strikes a superseded value and links in-page to its successor", async () => {
    await page("tab=verdicts", standard());
    const v1 = document.querySelector("#vrd_1")!;
    expect(v1.querySelector("s")?.textContent).toBe("fail");
    const link = [...v1.querySelectorAll("a")].find((a) => a.textContent === "superseded by vrd_2")!;
    expect(link.getAttribute("href")).toBe("#vrd_2");
  });

  it("offers Revise only on the viewer's own rubric verdicts", async () => {
    const own = async (path: string) => {
      if (path === API) return { ...doc, verdicts: [] };
      if (path.startsWith("/v1/verdicts?subject_kind=run")) return { verdicts: [hv("vrd_1", {}), hv("vrd_2", { standard: "stage:draft" })] };
      return { verdicts: [] };
    };
    await page("tab=verdicts", own, { ...me, principal: "prn_someone" });
    expect(button("Revise")).toBeUndefined();
    cleanup();
    await page("tab=verdicts", own);
    const revise = [...document.querySelectorAll("button")].filter((b) => b.textContent?.trim() === "Revise");
    expect(revise.map((b) => b.closest("li")?.id)).toEqual(["vrd_1"]);
  });

  it("Revise opens the drawer for that verdict", async () => {
    await page("tab=verdicts", standard());
    expect(document.querySelector("article[data-drawer]")).toBeNull();
    await click(button("Revise")!);
    expect(document.querySelector("article")?.getAttribute("data-drawer")).toBe("vrd_2");
  });

  it("links session-range evidence to the transcript, and not span ids", async () => {
    await page("tab=verdicts", async (path) => {
      if (path === API) return { ...doc, verdicts: [] };
      if (path.startsWith("/v1/verdicts?subject_kind=run"))
        return { verdicts: [hv("vrd_1", { evidence_refs: ["00f067aa0ba902bc", { session_id: "acps_a1", seq_from: 3, seq_to: 4 }, { session_id: "acps_zz", seq_from: 1, seq_to: 1 }] })] };
      return { verdicts: [] };
    });
    const links = [...document.querySelectorAll("#vrd_1 a")];
    expect(links.map((a) => a.textContent)).toEqual(["transcript 3–4 ↗"]);
    expect(links[0].getAttribute("href")).toBe(`${BASE}?tab=transcript&attempt=attempt_a1&seq=3-4`);
  });

  it("shows gates in words, never raw JSON", async () => {
    const gates = [
      { id: "gate_01", kind: "gate", standard: "stage:draft", status: "resolved", value: { decision: "rejected" }, judge: "prn_human02" },
      { id: "gate_02", kind: "gate", standard: "stage:review", status: "pending", value: { decision: null }, judge: "unknown" },
      { id: "gate_03", kind: "gate", standard: "stage:publish", status: "cancelled", value: null, judge: "unknown" },
    ];
    await page("tab=verdicts", async (path) => (path === API ? { ...doc, verdicts: gates } : { verdicts: [] }));
    const rows = [...document.querySelectorAll("li.gate")];
    expect(rows).toHaveLength(3);
    expect(rows[0].textContent).toContain("Gate");
    expect(rows[0].textContent).toContain("resolved");
    expect(rows[0].textContent).toContain("rejected");
    expect(rows[1].textContent).toContain("pending");
    expect(rows[1].textContent).toContain("no decision yet");
    expect(rows[2].textContent).toContain("cancelled");
    expect(rows[2].textContent).toContain("no decision yet");
    expect(text()).not.toContain("{");
    expect(text()).not.toContain("No verdicts yet.");
  });

  it("says when there are none", async () => {
    await page("tab=verdicts", async (path) => (path === API ? { ...doc, verdicts: [] } : { verdicts: [] }));
    expect(text()).toContain("No verdicts yet.");
  });

  it("says it is loading", async () => {
    await page("tab=verdicts", (path) => (path === API ? Promise.resolve({ ...doc, verdicts: [] }) : new Promise(() => {})));
    expect(text()).toContain("Loading verdicts…");
  });

  it("a failed verdict read says unavailable and offers a retry, never 'No verdicts yet'", async () => {
    let fail = true;
    await page("tab=verdicts", async (path) => {
      if (path === API) return { ...doc, verdicts: [] };
      if (path.startsWith("/v1/verdicts?subject_kind=attempt")) return { verdicts: [hv("vrd_3", { subject_kind: "attempt", subject_ref: "attempt_a1" })] };
      if (path.startsWith("/v1/verdicts?subject_kind=run")) {
        if (fail) throw new ApiError(503, "store_unavailable", "Can't reach the database", true);
        return { verdicts: [hv("vrd_1", {})] };
      }
      return {};
    });
    expect(text()).toContain("unavailable: the verdict history for this run could not be read (Can't reach the database).");
    expect(text()).not.toContain("No verdicts yet");
    expect(document.querySelector("#vrd_3")).not.toBeNull();
    fail = false;
    await click(button("Try again")!);
    await flush();
    expect(document.querySelector("#vrd_1")).not.toBeNull();
    expect(text()).not.toContain("unavailable: the verdict history");
  });

  it("only failed subjects are unavailable, and nothing else claims empty", async () => {
    await page("tab=verdicts", async (path) => {
      if (path === API) return { ...doc, verdicts: [] };
      throw new ApiError(0, "network", "superwitness could not be reached");
    });
    expect(text()).toContain("for this run");
    expect(text()).toContain("for attempt attempt_a1");
    expect(text()).not.toContain("No verdicts yet");
    expect(document.querySelector('[role="alert"]')).not.toBeNull();
  });
});
