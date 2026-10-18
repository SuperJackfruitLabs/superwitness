import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, getJSON, postJSON } from "./api";

function stub(res: Response | Error) {
  const f = vi.fn(async () => {
    if (res instanceof Error) throw res;
    return res;
  });
  vi.stubGlobal("fetch", f);
  return f;
}
const json = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });

async function refusal(p: Promise<unknown>): Promise<ApiError> {
  try {
    await p;
  } catch (e) {
    return e as ApiError;
  }
  throw new Error("expected a refusal");
}

afterEach(() => vi.unstubAllGlobals());

describe("request mapping", () => {
  it("returns the body of a success and sends cookies", async () => {
    const f = stub(json(200, { principal: "prn_human01" }));
    expect(await getJSON("/v1/me")).toEqual({ principal: "prn_human01" });
    expect(f).toHaveBeenCalledWith("/v1/me", expect.objectContaining({ method: "GET", credentials: "same-origin" }));
  });
  it("posts JSON", async () => {
    const f = stub(json(200, {}));
    await postJSON("/v1/verdicts", { a: 1 });
    expect(f).toHaveBeenCalledWith(
      "/v1/verdicts",
      expect.objectContaining({ method: "POST", body: '{"a":1}', headers: { "Content-Type": "application/json" } }),
    );
  });
  it("maps 401 and 403", async () => {
    stub(json(401, { error: { code: "unauthenticated", message: "sign in" } }));
    const a = await refusal(getJSON("/v1/me"));
    expect([a.status, a.code, a.message]).toEqual([401, "unauthenticated", "sign in"]);
    stub(json(403, { error: { code: "not_authorised", message: "no" } }));
    const b = await refusal(getJSON("/v1/me"));
    expect([b.status, b.code]).toEqual([403, "not_authorised"]);
  });
  it("keeps retry details of a 429", async () => {
    stub(json(429, { error: { code: "rate_limited", message: "slow", retryable: true } }, { "Retry-After": "7" }));
    const e = await refusal(getJSON("/v1/runs"));
    expect([e.status, e.code, e.retryable, e.retryAfter]).toEqual([429, "rate_limited", true, 7]);
  });
  it("maps a 503 store_unavailable", async () => {
    stub(json(503, { error: { code: "store_unavailable", message: "down", retryable: true } }));
    const e = await refusal(postJSON("/auth/logout"));
    expect([e.status, e.code, e.retryable, e.retryAfter]).toEqual([503, "store_unavailable", true, null]);
  });
  it("resolves a 204 to undefined", async () => {
    stub(new Response(null, { status: 204 }));
    expect(await postJSON("/auth/logout")).toBeUndefined();
  });
  it("calls a network failure status 0", async () => {
    stub(new TypeError("Failed to fetch"));
    const e = await refusal(getJSON("/v1/me"));
    expect([e.status, e.code]).toEqual([0, "network"]);
  });
  it("names a non-JSON refusal by its status", async () => {
    stub(new Response("<html>bad gateway</html>", { status: 502 }));
    const e = await refusal(getJSON("/v1/me"));
    expect([e.status, e.code, e.message]).toEqual([502, "http_502", "HTTP 502"]);
  });
});
