import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ApiError } from "./api";
import { ErrorView } from "./Edge";

const view = (e: ApiError) => renderToString(<ErrorView error={e} next="/runs?x=1" />);

describe("ErrorView", () => {
  it("sends a 401 to sign in, remembering where the person was", () => {
    const html = view(new ApiError(401, "unauthenticated", "x"));
    expect(html).toContain("Sign in with AgentPod");
    expect(html).toContain("/auth/login?next=%2Fruns%3Fx%3D1");
  });
  it("says not authorised", () => {
    expect(view(new ApiError(403, "not_authorised", "x"))).toContain("Not authorised");
  });
  it("says the database is down", () => {
    expect(view(new ApiError(503, "store_unavailable", "x"))).toContain("reach the database");
  });
  it("says the run is not found", () => {
    expect(view(new ApiError(404, "run_not_found", "x"))).toContain("Run not found");
  });
  it("tells a rate-limited person when to retry", () => {
    expect(view(new ApiError(429, "rate_limited", "x", true, 7))).toContain("Too many requests — try again in 7 s");
    expect(view(new ApiError(429, "rate_limited", "x", true, null))).toContain("try again shortly");
  });
  it("explains an unresolved principal and a mismatched origin", () => {
    expect(view(new ApiError(503, "principal_unresolved", "x", true))).toContain("AgentPod can’t be reached to confirm who you are");
    expect(view(new ApiError(403, "origin_mismatch", "x"))).toContain("Reload the page");
  });
  it("falls back to the server's message", () => {
    expect(view(new ApiError(500, "internal", "it broke"))).toContain("it broke");
  });
});
