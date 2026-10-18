import { describe, expect, it } from "vitest";
import { sourcesState } from "./sources";

const ok = { status: "ok", version: "x", sources: { a: "ok", b: "ok", c: "ok", d: "ok", e: "ok" } };

describe("sourcesState", () => {
  it("is green only when all answer", () => {
    expect(sourcesState(ok).tone).toBe("good");
    expect(sourcesState({ ...ok, sources: { ...ok.sources, c: "timeout" } })).toEqual({ tone: "warn", label: "c: timeout" });
    expect(sourcesState(null).tone).toBe("warn");
  });
  it("is amber when the store is unavailable, whatever /health says", () => {
    expect(sourcesState(ok, true).tone).toBe("warn");
  });
});
