import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/svelte";
import { ApiError } from "../api";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({ ...(await orig<typeof import("../api")>()), getJSON: (...a: unknown[]) => getJSON(...a) }));
const goto = vi.fn();
vi.mock("$app/navigation", () => ({ goto: (...a: unknown[]) => goto(...a) }));
import RunView from "./RunView.svelte";

const API = "/v1/runs/superpipeline/brd_01/run_01";
const BASE = "/runs/superpipeline/brd_01/run_01";
const sources = { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "ok", errors: "ok", verdicts: "ok" };
const doc = {
  run: { ref: "superpipeline:brd_01/run_01", agent: "prn_agent01", state: "completed", started_at: "2026-10-04T10:00:00Z", ended_at: "2026-10-04T10:05:00Z", card: { title: "Write the release note" } },
  attempts: [{ id: "attempt_01" }],
  errors: [{ at: "t", service: "agentpod-hub", message: "x" }],
  cost: { status: "unreported" },
  verdicts: [],
  sources,
};

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const mount = async (query = "", d: object = doc, board = "brd_01", run = "run_01") => {
  getJSON.mockImplementation(async (path: string) => (path === `/v1/runs/superpipeline/${board}/${run}` ? d : { spans: [], logs: [], verdicts: [] }));
  const v = render(RunView, { board, run, query });
  await flush();
  await flush();
  return v;
};
const facts = () => Object.fromEntries([...document.querySelectorAll(".sw-facts > div")].map((d) => [d.querySelector("dt")!.textContent, d.querySelector("dd")!.textContent]));

beforeEach(() => {
  getJSON.mockReset();
  goto.mockReset();
});
afterEach(cleanup);

describe("the run header", () => {
  it("names the run, its title, its state in its tone and offers Record verdict", async () => {
    await mount();
    expect(getJSON).toHaveBeenCalledWith(API);
    expect(document.querySelector(".sw-runhead__ref")?.textContent).toBe("superpipeline · brd_01/run_01");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Write the release note");
    const pill = document.querySelector(".sw-pill")!;
    expect(pill.textContent).toContain("completed");
    expect(pill.getAttribute("data-tone")).toBe("success");
    expect(screen.getByRole("button", { name: "Record verdict" })).toBeTruthy();
    expect(document.querySelectorAll("h1")).toHaveLength(1);
  });

  it("falls back to board/run when the title is unknown", async () => {
    await mount("", { ...doc, run: { ...doc.run, card: { title: "unknown" } } });
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("brd_01/run_01");
  });

  it("says unknown for a state the run does not report", async () => {
    await mount("", { ...doc, run: {} });
    expect(document.querySelector(".sw-pill")?.textContent).toContain("unknown");
  });
});

describe("the facts", () => {
  it("shows the run's figures", async () => {
    await mount();
    expect(facts()).toEqual({ Agent: "prn_agent01", Attempts: "1", Errors: "1", Duration: "5m 0s", Cost: "unreported" });
  });

  it("shows unknown facts as unknown, never 0, and marks the deliberate words", async () => {
    await mount("", { run: {}, sources: {}, cost: {} });
    expect(facts()).toEqual({ Agent: "unknown", Attempts: "unknown", Errors: "unknown", Duration: "unknown", Cost: "unknown" });
    expect([...document.querySelectorAll(".sw-facts dd")].every((d) => d.classList.contains("sw-word"))).toBe(true);
  });

  it("says in progress for a running run and the cost when reported", async () => {
    await mount("", { ...doc, run: { ...doc.run, ended_at: null }, cost: { status: "reported", usd: 1.234 } });
    expect(facts().Duration).toBe("in progress");
    expect(facts().Cost).toBe("$1.23");
    const dd = [...document.querySelectorAll(".sw-facts dd")];
    expect(dd[3].classList.contains("sw-word")).toBe(true);
    expect(dd[4].classList.contains("sw-word")).toBe(false);
  });

  it("draws the agent principal in mono, free to wrap inside its fact", async () => {
    const long = "prn_agent_0123456789abcdef0123456789abcdef0123456789abcdef";
    await mount("", { ...doc, run: { ...doc.run, agent: long } });
    const dd = document.querySelector(".sw-facts > div:first-child dd")!;
    expect(dd.textContent).toBe(long);
    expect(dd.classList.contains("sw-mono")).toBe(true);
  });
});

describe("the tabs", () => {
  it("opens the tab named in the URL, and each tab is a URL with trace left out", async () => {
    await mount("tab=attempts");
    expect(document.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toBe("Attempts");
    const hrefs = screen.getAllByRole("tab").map((t) => [t.textContent, t.getAttribute("href")]);
    expect(hrefs).toEqual([
      ["Trace", BASE],
      ["Transcript", `${BASE}?tab=transcript`],
      ["Logs", `${BASE}?tab=logs`],
      ["Errors", `${BASE}?tab=errors`],
      ["Verdicts", `${BASE}?tab=verdicts`],
      ["Attempts", `${BASE}?tab=attempts`],
    ]);
    expect(screen.getByRole("tablist", { name: "Run" })).toBeTruthy();
    expect(screen.getByRole("tabpanel", { name: "Attempts" })).toBeTruthy();
  });

  it("an unknown tab is the Trace tab", async () => {
    await mount("tab=nonsense");
    expect(document.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toBe("Trace");
  });

  it("arrow keys move to the next tab's URL", async () => {
    await mount("");
    const tabs = screen.getAllByRole("tab");
    tabs[0].dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    expect(goto.mock.calls[0][0]).toBe(`${BASE}?tab=transcript`);
  });

  const cases: [string, string, string][] = [
    ["", "traces", "Trace"],
    ["tab=transcript", "agentpod", "Transcript"],
    ["tab=logs", "logs", "Logs"],
    ["tab=errors", "errors", "Errors"],
    ["tab=verdicts", "verdicts", "Verdicts"],
    ["tab=attempts", "agentpod", "Attempts"],
  ];
  for (const [query, source, label] of cases) {
    it(`says the ${label} tab's source (${source}) is unavailable instead of showing it empty`, async () => {
      await mount(query, { ...doc, sources: { ...sources, [source]: "timeout" } });
      const panel = screen.getByRole("tabpanel", { name: label });
      expect(panel.textContent?.trim()).toBe(`unavailable: the ${source} source did not answer (timeout).`);
    });
  }

  it("says unknown when the source sent no status", async () => {
    await mount("tab=logs", { ...doc, sources: { ...sources, logs: undefined } });
    expect(screen.getByRole("tabpanel", { name: "Logs" }).textContent?.trim()).toBe("unavailable: the logs source did not answer (unknown).");
  });

  it("keeps the facts while a tab's source is unavailable", async () => {
    await mount("tab=logs", { ...doc, sources: { ...sources, logs: "timeout" } });
    expect(facts().Agent).toBe("prn_agent01");
  });
});

describe("the run's own states", () => {
  it("says Run not found inside the shell", async () => {
    getJSON.mockRejectedValue(new ApiError(404, "run_not_found", "no such run"));
    render(RunView, { board: "brd_01", run: "run_99", query: "" });
    await flush();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Run not found");
    expect(screen.getByRole("link", { name: "All runs" }).getAttribute("href")).toBe("/");
  });

  it("says Loading… while the run is read", async () => {
    getJSON.mockImplementation(() => new Promise(() => {}));
    render(RunView, { board: "brd_01", run: "run_01", query: "" });
    await flush();
    expect(document.body.textContent).toContain("Loading…");
  });
});
