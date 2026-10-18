import { describe, expect, it } from "vitest";
import { apiQuery, viewTitle, withStatus } from "./filters";

describe("filters", () => {
  it("passes only the API's own parameters", () => {
    expect(apiQuery(new URLSearchParams("status=failed&status=waiting&tab=x&scope=brd_01"))).toBe("scope=brd_01&status=failed&status=waiting");
  });
  it("names each view", () => {
    expect(viewTitle(new URLSearchParams(""))).toBe("All runs");
    expect(viewTitle(new URLSearchParams("needs_verdict=true"))).toBe("Needs verdict");
    expect(viewTitle(new URLSearchParams("status=failed"))).toBe("Failed");
    expect(viewTitle(new URLSearchParams("status=waiting"))).toBe("Waiting");
    expect(viewTitle(new URLSearchParams("scope=brd_01"))).toBe("Board brd_01");
    expect(viewTitle(new URLSearchParams("status=failed&executor=prn_agent01"))).toBe("Runs");
  });
  it("toggles a status chip", () => {
    expect(withStatus(new URLSearchParams("scope=brd_01"), "failed")).toBe("/?scope=brd_01&status=failed");
    expect(withStatus(new URLSearchParams("status=failed"), "failed")).toBe("/");
  });
});
