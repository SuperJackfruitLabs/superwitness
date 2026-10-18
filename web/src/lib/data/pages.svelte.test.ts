import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { flushSync } from "svelte";
import { ApiError } from "../api";
import { pages } from "./pages.svelte";

interface Page {
  items: string[];
  next: string | null;
  total?: number;
}
let calls: { cursor: string | null; resolve: (p: Page) => void; reject: (e: ApiError) => void }[];
let stop: (() => void) | null;
const fetchPage = (cursor: string | null) => new Promise<Page>((resolve, reject) => calls.push({ cursor, resolve, reject }));
const settle = async () => {
  await new Promise((r) => setTimeout(r, 0));
  flushSync();
};
function mount(key: () => string) {
  let p!: ReturnType<typeof pages<string, Page>>;
  stop = $effect.root(() => {
    p = pages<string, Page>(key, fetchPage);
  });
  flushSync();
  return p;
}
const err = (status = 500) => new ApiError(status, "boom", "boom");

beforeEach(() => {
  calls = [];
  stop = null;
});
afterEach(() => stop?.());

describe("pages", () => {
  it("loads the first page, then appends the second", async () => {
    const p = mount(() => "k");
    expect(p.loading).toBe(true);
    expect(calls.map((c) => c.cursor)).toEqual([null]);
    calls[0].resolve({ items: ["a", "b"], next: "c1", total: 3 });
    await settle();
    expect(p.loading).toBe(false);
    expect(p.items).toEqual(["a", "b"]);
    expect(p.next).toBe("c1");
    expect(p.first?.total).toBe(3);
    p.more();
    expect(calls.map((c) => c.cursor)).toEqual([null, "c1"]);
    p.more(); // already loading more: ignored
    expect(calls).toHaveLength(2);
    calls[1].resolve({ items: ["c"], next: null });
    await settle();
    expect(p.items).toEqual(["a", "b", "c"]);
    expect(p.next).toBeNull();
    expect(p.first?.total).toBe(3);
    p.more(); // nothing left
    expect(calls).toHaveLength(2);
  });

  it("a first-load failure is error; retry loads again", async () => {
    const p = mount(() => "k");
    calls[0].reject(err());
    await settle();
    expect(p.error?.status).toBe(500);
    expect(p.items).toEqual([]);
    expect(p.loading).toBe(false);
    p.retry();
    flushSync();
    expect(calls).toHaveLength(2);
    expect(p.error).toBeNull();
    expect(p.loading).toBe(true);
    calls[1].resolve({ items: ["a"], next: null });
    await settle();
    expect(p.items).toEqual(["a"]);
  });

  it("a load-more failure is moreError and keeps the rows", async () => {
    const p = mount(() => "k");
    calls[0].resolve({ items: ["a"], next: "c1" });
    await settle();
    p.more();
    calls[1].reject(err(503));
    await settle();
    expect(p.moreError?.status).toBe(503);
    expect(p.error).toBeNull();
    expect(p.items).toEqual(["a"]);
    expect(p.next).toBe("c1");
    p.more(); // trying again is allowed and clears the error
    expect(p.moreError).toBeNull();
    expect(calls).toHaveLength(3);
    calls[2].resolve({ items: ["b"], next: null });
    await settle();
    expect(p.items).toEqual(["a", "b"]);
  });

  it("a new key resets and loads the first page again", async () => {
    let key = $state("k1");
    const p = mount(() => key);
    calls[0].resolve({ items: ["a"], next: "c1" });
    await settle();
    key = "k2";
    flushSync();
    expect(p.items).toEqual([]);
    expect(p.next).toBeNull();
    expect(p.loading).toBe(true);
    expect(calls.map((c) => c.cursor)).toEqual([null, null]);
    calls[1].resolve({ items: ["x"], next: null });
    await settle();
    expect(p.items).toEqual(["x"]);
  });

  it("drops a page 2 that answers after the key changed", async () => {
    let key = $state("k1");
    const p = mount(() => key);
    calls[0].resolve({ items: ["a"], next: "c1" });
    await settle();
    p.more();
    key = "k2";
    flushSync();
    calls[2].resolve({ items: ["x"], next: null });
    await settle();
    calls[1].resolve({ items: ["STALE"], next: "c9" });
    await settle();
    expect(p.items).toEqual(["x"]);
    expect(p.next).toBeNull();
    // and a stale failure is dropped too
    expect(p.moreError).toBeNull();
  });

  it("drops a stale first page after retry", async () => {
    const p = mount(() => "k");
    p.retry();
    flushSync();
    calls[1].resolve({ items: ["new"], next: null });
    await settle();
    calls[0].resolve({ items: ["old"], next: "c1" });
    await settle();
    expect(p.items).toEqual(["new"]);
    expect(p.next).toBeNull();
  });
});
