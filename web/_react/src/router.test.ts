import { describe, expect, it } from "vitest";
import { match } from "./router";
import { sourcesState } from "./sources";

describe("match", () => {
  it("routes each page", () => {
    expect(match("/")).toEqual({ name: "runs" });
    expect(match("/runs/superpipeline/brd_01/run_01")).toEqual({ name: "run", board: "brd_01", run: "run_01" });
    expect(match("/rubrics")).toEqual({ name: "rubrics" });
    expect(match("/rubrics/press/2")).toEqual({ name: "rubric", id: "press", version: 2 });
    expect(match("/runs/superpipeline/brd 1/run_01")).toEqual({ name: "notfound" });
    expect(match("/rubrics/press/0")).toEqual({ name: "notfound" });
  });
});

describe("sources dot", () => {
  it("is good only when every source answers", () => {
    expect(sourcesState({ status: "ok", version: "v", sources: { superpipeline: "ok", verdicts: "ok" } }).tone).toBe("good");
    expect(sourcesState({ status: "ok", version: "v", sources: { superpipeline: "ok", traces: "timeout" } })).toEqual({ tone: "warn", label: "traces: timeout" });
    expect(sourcesState(null).tone).toBe("warn");
  });
});
