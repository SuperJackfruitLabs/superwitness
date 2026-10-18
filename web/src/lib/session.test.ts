import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getJSON, postJSON } from "./api";
import { session, signInHref } from "./session.svelte";

const answer = (status: number) =>
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: { code: status === 401 ? "unauthenticated" : "x", message: "m" } }), { status })));

beforeEach(() => {
  session.expired = false;
});
afterEach(() => vi.unstubAllGlobals());

describe("session", () => {
  it("a 401 from any GET marks the session expired", async () => {
    answer(401);
    await expect(getJSON("/v1/runs")).rejects.toMatchObject({ status: 401 });
    expect(session.expired).toBe(true);
  });
  it("a 401 from a POST (the verdict drawer) marks it expired too", async () => {
    answer(401);
    await expect(postJSON("/v1/verdicts", {})).rejects.toMatchObject({ status: 401 });
    expect(session.expired).toBe(true);
  });
  it("other refusals do not", async () => {
    answer(403);
    await expect(getJSON("/v1/runs")).rejects.toMatchObject({ status: 403 });
    answer(503);
    await expect(getJSON("/v1/runs")).rejects.toMatchObject({ status: 503 });
    expect(session.expired).toBe(false);
  });
  it("signInHref carries the current path and query", () => {
    expect(signInHref("/runs/superpipeline/b/r", "tab=logs&seq=3-4")).toBe("/auth/login?next=%2Fruns%2Fsuperpipeline%2Fb%2Fr%3Ftab%3Dlogs%26seq%3D3-4");
    expect(signInHref("/rubrics", "")).toBe("/auth/login?next=%2Frubrics");
  });
});
