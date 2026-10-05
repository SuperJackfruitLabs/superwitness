import { describe, expect, it } from "vitest";
import { display, parseRunPath, statusTone } from "./format";

describe("parseRunPath", () => {
  it("reads board and run", () => {
    expect(parseRunPath("/runs/superpipeline/brd_01/run_01")).toEqual({ board: "brd_01", run: "run_01" });
    expect(parseRunPath("/runs/superpipeline/brd_01/run_01/")).toEqual({ board: "brd_01", run: "run_01" });
  });
  it("rejects anything else", () => {
    expect(parseRunPath("/")).toBeNull();
    expect(parseRunPath("/runs/superpipeline/brd_01")).toBeNull();
    expect(parseRunPath("/runs/superpipeline/brd 1/run_01")).toBeNull();
  });
});

describe("display", () => {
  it("never shows a missing value as zero or blank", () => {
    expect(display(null)).toBe("—");
    expect(display(undefined)).toBe("—");
    expect(display("unknown")).toBe("unknown");
    expect(display(0)).toBe("0");
    expect(display({ score: 0.7 })).toBe('{"score":0.7}');
  });
});

describe("statusTone", () => {
  it("maps source statuses", () => {
    expect(statusTone("ok")).toBe("good");
    expect(statusTone("not_found")).toBe("warn");
    expect(statusTone("timeout")).toBe("warn");
    expect(statusTone("unavailable")).toBe("bad");
    expect(statusTone("unauthorized")).toBe("bad");
  });
});
