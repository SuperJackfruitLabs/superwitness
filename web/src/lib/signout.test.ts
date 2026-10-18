import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "./api";
import { signOut, signOutReason } from "./signout.svelte";

const j = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
const answer = (f: () => Response) => vi.stubGlobal("fetch", vi.fn(async () => f()));

afterEach(() => vi.unstubAllGlobals());

describe("signOutReason", () => {
  it.each([
    [new ApiError(429, "rate_limited", "x", true, 7), "Too many requests; wait 7 s."],
    [new ApiError(429, "rate_limited", "x", true, null), "Too many requests; wait a moment."],
    [new ApiError(0, "network", "superwitness could not be reached"), "superwitness could not be reached."],
    [new ApiError(403, "origin_mismatch", "x"), "The request didn’t come from the app; reload the page first."],
    [new ApiError(500, "internal", "it broke"), "it broke"],
    [new Error("boom"), "Something went wrong."],
    [new ApiError(502, "http_502", ""), "The server answered HTTP 502."],
  ])("%s -> %s", (e, want) => expect(signOutReason(e)).toBe(want));
});

describe("signOut", () => {
  it("204 loads / in full", async () => {
    answer(() => new Response(null, { status: 204 }));
    const assign = vi.fn();
    const out = signOut(assign);
    await out.run();
    expect(assign).toHaveBeenCalledWith("/");
  });
  it("503 store_unavailable is signed out here, lingering on the server", async () => {
    answer(() => j(503, { error: { code: "store_unavailable", message: "down" } }));
    const assign = vi.fn();
    const out = signOut(assign);
    await out.run();
    expect(assign).not.toHaveBeenCalled();
    expect(out.state).toEqual({ kind: "lingering" });
  });
  it("is busy while the request is out, then says why it failed", async () => {
    let release!: (r: Response) => void;
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((r) => (release = r))));
    const out = signOut(vi.fn());
    const p = out.run();
    expect(out.state.kind).toBe("busy");
    release(j(429, { error: { code: "rate_limited", message: "slow" } }, { "Retry-After": "7" }));
    await p;
    expect(out.state).toEqual({ kind: "failed", reason: "Too many requests; wait 7 s." });
  });
});
