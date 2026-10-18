import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, within } from "@testing-library/svelte";
import { ApiError } from "../api";
import type { RegistryRun, RunPage } from "../types";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({ ...(await orig<typeof import("../api")>()), getJSON: (...a: unknown[]) => getJSON(...a) }));
import RunsPage from "./RunsPage.svelte";

const run = (ref: string, over: Partial<RegistryRun> = {}): RegistryRun =>
  ({ source: "canary", external_ref: ref, ref, scope: null, title: `run ${ref}`, executor: null, status: "succeeded", source_status: "ok",
    started_at: new Date().toISOString(), ended_at: null, reported_at: "", first_seen_at: new Date().toISOString(), updated_at: "", latest_verdict: null, ...over }) as RegistryRun;
const counts = { queued: 0, running: 1, waiting: 1, succeeded: 5, failed: 2, cancelled: 0 };
const page = (refs: string[], next: string | null, runs?: RegistryRun[]): RunPage => ({ runs: runs ?? refs.map((r) => run(r)), next_cursor: next, counts });

// A fake IntersectionObserver that records every observer made, so a test can see re-creation.
let observers: { cb: () => void; live: boolean }[] = [];
class FakeIO {
  o = { cb: () => {}, live: true };
  constructor(cb: (e: { isIntersecting: boolean }[]) => void) {
    this.o.cb = () => cb([{ isIntersecting: true }]);
    observers.push(this.o);
  }
  observe() {}
  disconnect() {
    this.o.live = false;
  }
}

const deferred = <T>() => {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((a, b) => ((resolve = a), (reject = b)));
  return { promise, resolve, reject };
};
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

let view: ReturnType<typeof render<typeof RunsPage>>;
const text = () => document.body.textContent ?? "";
const mount = async (query: string) => {
  view = render(RunsPage, { query });
  await flush();
};
const requery = async (query: string) => {
  await view.rerender({ query });
};
const button = () => screen.queryAllByRole("button").find((b) => /Load more|Loading…/.test(b.textContent ?? "")) as HTMLButtonElement | undefined;

beforeEach(() => {
  getJSON.mockReset();
  observers = [];
  vi.stubGlobal("IntersectionObserver", FakeIO);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("RunsPage paging", () => {
  it("pages with next_cursor until it is null", async () => {
    getJSON.mockResolvedValueOnce(page(["a"], "c1")).mockResolvedValueOnce(page(["b"], null));
    await mount("");
    expect(getJSON).toHaveBeenCalledWith("/v1/runs");
    expect(text()).toContain("run a");
    await act(async () => button()!.click());
    await flush();
    expect(getJSON).toHaveBeenLastCalledWith("/v1/runs?cursor=c1");
    expect(text()).toContain("run b");
    expect(button()).toBeUndefined();
  });

  it("keeps the filter on the next page's request", async () => {
    getJSON.mockResolvedValueOnce(page(["a"], "c 1")).mockResolvedValueOnce(page(["b"], null));
    await mount("status=failed&x=1");
    expect(getJSON).toHaveBeenCalledWith("/v1/runs?status=failed");
    await act(async () => button()!.click());
    await flush();
    expect(getJSON).toHaveBeenLastCalledWith("/v1/runs?status=failed&cursor=c%201");
  });

  it("says Loading… on a disabled button while the next page is in flight", async () => {
    const late = deferred<RunPage>();
    getJSON.mockResolvedValueOnce(page(["a"], "c1")).mockReturnValueOnce(late.promise);
    await mount("");
    await act(async () => button()!.click());
    expect(button()!.textContent).toContain("Loading…");
    expect(button()!.disabled).toBe(true);
    await act(async () => late.resolve(page(["b"], null)));
    await flush();
    expect(button()).toBeUndefined();
  });

  it("discards a load-more still in flight when the filter changes", async () => {
    const late = deferred<RunPage>();
    getJSON.mockResolvedValueOnce(page(["a"], "c1")).mockReturnValueOnce(late.promise).mockResolvedValueOnce(page(["z"], null));
    await mount("");
    await act(async () => button()!.click());
    await requery("status=failed");
    await flush();
    expect(text()).toContain("run z");
    await act(async () => late.resolve(page(["stale"], null)));
    await flush();
    expect(text()).not.toContain("run stale");
    expect(text()).toContain("run z");
  });

  it("ignores a stale load-more failure after the filter changed", async () => {
    const late = deferred<RunPage>();
    const fresh = deferred<RunPage>();
    getJSON.mockResolvedValueOnce(page(["a"], "c1")).mockReturnValueOnce(late.promise).mockReturnValueOnce(fresh.promise);
    await mount("");
    await act(async () => button()!.click());
    await requery("status=failed");
    await act(async () => late.reject(new ApiError(503, "x", "old filter broke")));
    await flush();
    expect(text()).not.toContain("old filter broke");
    await act(async () => fresh.resolve(page(["z"], null)));
    await flush();
    expect(text()).toContain("run z");
  });

  it("does not retry by itself after an error; the button retries and clears the banner", async () => {
    getJSON
      .mockResolvedValueOnce(page(["a"], "c1"))
      .mockRejectedValueOnce(new ApiError(503, "store_unavailable", "database is down", true))
      .mockResolvedValueOnce(page(["b"], null));
    await mount("");
    await act(async () => observers.filter((o) => o.live).forEach((o) => o.cb()));
    await flush();
    expect(text()).toContain("database is down");
    expect(text()).toContain("run a");
    expect(getJSON).toHaveBeenCalledTimes(2);
    // The failed state has no live observer, so nothing can fire a retry.
    expect(observers.filter((o) => o.live)).toHaveLength(0);
    await flush();
    expect(getJSON).toHaveBeenCalledTimes(2);
    await act(async () => button()!.click());
    await flush();
    expect(getJSON).toHaveBeenCalledTimes(3);
    expect(text()).toContain("run b");
    expect(text()).not.toContain("database is down");
  });
});

describe("RunsPage view", () => {
  it.each([
    ["", "All runs"],
    ["needs_verdict=true", "Needs verdict"],
    ["scope=brd_01", "Board brd_01"],
    ["status=failed", "Failed"],
    ["status=waiting", "Waiting"],
    ["status=running", "Runs"],
  ])("titles ?%s as %s", async (q, title) => {
    getJSON.mockResolvedValue(page(["a"], null));
    await mount(q);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(title);
  });

  it("draws six status chips in order with the first page's counts, linked to that status", async () => {
    getJSON.mockResolvedValueOnce(page(["a"], "c1")).mockResolvedValueOnce({ ...page(["b"], null), counts: { ...counts, failed: 99 } });
    await mount("scope=brd_01");
    const chips = within(screen.getByRole("list", { name: "Runs by status" })).getAllByRole("link");
    expect(chips.map((a) => a.textContent?.replace(/\s+/g, " ").trim())).toEqual([
      "queued · 0", "running · 1", "waiting · 1", "succeeded · 5", "failed · 2", "cancelled · 0",
    ]);
    expect(chips[4]).toHaveAttribute("href", "/?scope=brd_01&status=failed");
    expect(chips.some((a) => a.hasAttribute("aria-current"))).toBe(false);
    await act(async () => button()!.click());
    await flush();
    expect(chips[4].textContent).toContain("2");
  });

  it("marks the one selected status, whose chip clears it", async () => {
    getJSON.mockResolvedValue(page(["a"], null));
    await mount("status=failed");
    const chips = within(screen.getByRole("list", { name: "Runs by status" })).getAllByRole("link");
    expect(chips.filter((a) => a.getAttribute("aria-current") === "true")).toEqual([chips[4]]);
    expect(chips[4]).toHaveAttribute("href", "/");
    expect(chips[1]).toHaveAttribute("href", "/?status=running");
  });

  it("selects no chip when two statuses are in the URL", async () => {
    getJSON.mockResolvedValue(page(["a"], null));
    await mount("status=failed&status=waiting");
    const chips = within(screen.getByRole("list", { name: "Runs by status" })).getAllByRole("link");
    expect(chips.some((a) => a.hasAttribute("aria-current"))).toBe(false);
  });

  it("groups runs by day: Today, Yesterday, a date, Unknown date", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 9, 15, 12, 0));
    const at = (d: number) => new Date(2026, 9, d, 9, 0).toISOString();
    getJSON.mockResolvedValue(
      page([], null, [
        run("t", { started_at: at(15) }),
        run("y", { started_at: at(14) }),
        run("o", { started_at: at(13) }),
        run("u", { started_at: null, first_seen_at: "not a time" }),
      ]),
    );
    await mount("");
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual(["Today", "Yesterday", "Tue, 13 Oct 2026", "Unknown date"]);
    const today = screen.getByRole("region", { name: "Today" });
    expect(within(today).getByText("run t")).toBeTruthy();
  });

  it("says no runs reported yet, with the docs link, when nothing is filtered", async () => {
    getJSON.mockResolvedValue({ runs: [], next_cursor: null, counts });
    await mount("");
    expect(text()).toContain("No runs reported yet.");
    expect(screen.getByRole("link", { name: "How sources report runs" })).toHaveAttribute("href", "https://docs.superwitness.dev/build/run-registry/");
  });

  it("says no runs in this view when filtered", async () => {
    getJSON.mockResolvedValue({ runs: [], next_cursor: null, counts });
    await mount("status=failed");
    expect(text()).toContain("No runs in this view.");
    expect(text()).not.toContain("No runs reported yet.");
  });

  it("says Loading… with the heading and no chips while the first page loads", async () => {
    getJSON.mockReturnValue(new Promise(() => {}));
    await mount("");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("All runs");
    expect(screen.getByRole("status").textContent).toContain("Loading…");
    expect(screen.queryByRole("list", { name: "Runs by status" })).toBeNull();
    expect(text()).not.toContain("No runs");
  });

  it.each([
    [new ApiError(503, "store_unavailable", "db"), "Can’t reach the database"],
    [new ApiError(429, "rate_limited", "slow", true, 5), "Too many requests"],
    [new ApiError(500, "internal", "boom"), "boom"],
  ])("turns a first-page refusal into its edge page (%s)", async (err, want) => {
    getJSON.mockRejectedValue(err);
    await mount("");
    expect(text()).toContain(want);
    expect(screen.queryByRole("list", { name: "Runs by status" })).toBeNull();
  });
});
