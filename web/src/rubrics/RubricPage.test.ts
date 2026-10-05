import { describe, expect, it } from "vitest";
import { describeScale } from "./RubricPage";

describe("describeScale", () => {
  it("says each scale", () => {
    expect(describeScale({ kind: "decision", options: ["pass", "fail"] })).toBe("A decision: pass, fail.");
    expect(describeScale({ kind: "score", min: 1, max: 5 })).toBe("A score from 1 to 5, recorded as 0 to 1.");
    expect(describeScale(null)).toBe("Not supported in the app.");
  });
});
