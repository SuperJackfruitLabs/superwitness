// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api";
import type { RegistryRun, RunPage } from "../types";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({ ...(await orig<typeof import("../api")>()), getJSON: (...a: unknown[]) => getJSON(...a) }));
import { RunsPage } from "./RunsPage";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const run = (ref: string): RegistryRun =>
  ({ source: "canary", external_ref: ref, ref, scope: null, title: `run ${ref}`, executor: null, status: "succeeded", source_status: "ok",
    started_at: new Date().toISOString(), ended_at: null, reported_at: "", first_seen_at: new Date().toISOString(), updated_at: "", latest_verdict: null }) as RegistryRun;
const counts = { queued: 0, running: 0, waiting: 0, succeeded: 1, failed: 0, cancelled: 0 };
const page = (refs: string[], next: string | null): RunPage => ({ runs: refs.map(run), next_cursor: next, counts });

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

const deferred = <T,>() => {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((a, b) => ((resolve = a), (reject = b)));
  return { promise, resolve, reject };
};
const flush = () => act(async () => {});

let host: HTMLElement;
let root: Root;
const text = () => host.textContent ?? "";
const mount = (q: string) => act(async () => root.render(<RunsPage query={q} />));
const button = () => [...host.querySelectorAll("button")].find((b) => b.textContent?.includes("Load more")) as HTMLButtonElement | undefined;

beforeEach(() => {
  getJSON.mockReset();
  observers = [];
  vi.stubGlobal("IntersectionObserver", FakeIO);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

describe("RunsPage paging", () => {
  it("pages with next_cursor until it is null", async () => {
    getJSON.mockResolvedValueOnce(page(["a"], "c1")).mockResolvedValueOnce(page(["b"], null));
    await mount("");
    await flush();
    expect(text()).toContain("run a");
    await act(async () => button()!.click());
    await flush();
    expect(getJSON).toHaveBeenLastCalledWith("/v1/runs?cursor=c1");
    expect(text()).toContain("run b");
    expect(button()).toBeUndefined();
  });

  it("discards a load-more still in flight when the filter changes", async () => {
    const late = deferred<RunPage>();
    getJSON.mockResolvedValueOnce(page(["a"], "c1")).mockReturnValueOnce(late.promise).mockResolvedValueOnce(page(["z"], null));
    await mount("");
    await flush();
    await act(async () => button()!.click());
    await mount("status=failed");
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
    await flush();
    await act(async () => button()!.click());
    await mount("status=failed");
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
    await flush();
    await act(async () => observers.filter((o) => o.live).forEach((o) => o.cb()));
    await flush();
    expect(text()).toContain("database is down");
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
