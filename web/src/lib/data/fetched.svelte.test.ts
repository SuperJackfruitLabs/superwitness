import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushSync } from "svelte";
import { fetched } from "./fetched.svelte";

let calls: { path: string; resolve: (r: Response) => void }[];
let stop: (() => void) | null;
const ok = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });
const fail = (status: number, code: string) => new Response(JSON.stringify({ error: { code, message: code } }), { status });
const settle = async () => {
  await new Promise((r) => setTimeout(r, 0));
  flushSync();
};
function mount<T>(path: () => string | null) {
  let f!: ReturnType<typeof fetched<T>>;
  stop = $effect.root(() => {
    f = fetched<T>(path);
  });
  flushSync();
  return f;
}

beforeEach(() => {
  calls = [];
  stop = null;
  vi.stubGlobal(
    "fetch",
    vi.fn((path: string) => new Promise<Response>((resolve) => calls.push({ path, resolve }))),
  );
});
afterEach(() => {
  stop?.();
  vi.unstubAllGlobals();
});

describe("fetched", () => {
  it("is loading, then has data", async () => {
    const f = mount<{ n: number }>(() => "/v1/x");
    expect(f.loading).toBe(true);
    expect(f.data).toBeNull();
    expect(calls.map((c) => c.path)).toEqual(["/v1/x"]);
    calls[0].resolve(ok({ n: 1 }));
    await settle();
    expect(f.loading).toBe(false);
    expect(f.data).toEqual({ n: 1 });
    expect(f.error).toBeNull();
  });

  it("has an error, and no data, when the call fails", async () => {
    const f = mount(() => "/v1/x");
    calls[0].resolve(fail(503, "store_unavailable"));
    await settle();
    expect(f.loading).toBe(false);
    expect(f.data).toBeNull();
    expect(f.error?.status).toBe(503);
    expect(f.error?.code).toBe("store_unavailable");
  });

  it("fetches nothing for a null path", async () => {
    const f = mount(() => null);
    await settle();
    expect(calls).toHaveLength(0);
    expect(f.loading).toBe(false);
    expect(f.data).toBeNull();
  });

  it("reload fetches again and clears the error while it does", async () => {
    const f = mount(() => "/v1/x");
    calls[0].resolve(fail(500, "boom"));
    await settle();
    expect(f.error).not.toBeNull();
    f.reload();
    flushSync();
    expect(calls).toHaveLength(2);
    expect(f.loading).toBe(true);
    expect(f.error).toBeNull();
    calls[1].resolve(ok({ n: 2 }));
    await settle();
    expect(f.data).toEqual({ n: 2 });
  });

  it("ignores a slow answer for a path that has been replaced", async () => {
    let path = $state("/v1/a");
    const f = mount<{ from: string }>(() => path);
    expect(calls.map((c) => c.path)).toEqual(["/v1/a"]);
    path = "/v1/b";
    flushSync();
    expect(calls.map((c) => c.path)).toEqual(["/v1/a", "/v1/b"]);
    calls[1].resolve(ok({ from: "b" }));
    await settle();
    expect(f.data).toEqual({ from: "b" });
    calls[0].resolve(ok({ from: "a" }));
    await settle();
    expect(f.data).toEqual({ from: "b" });
    expect(f.loading).toBe(false);
  });

  it("ignores a slow failure for a path that has been replaced", async () => {
    let path = $state("/v1/a");
    const f = mount<{ from: string }>(() => path);
    path = "/v1/b";
    flushSync();
    calls[1].resolve(ok({ from: "b" }));
    await settle();
    calls[0].resolve(fail(500, "late"));
    await settle();
    expect(f.error).toBeNull();
    expect(f.data).toEqual({ from: "b" });
  });
});
