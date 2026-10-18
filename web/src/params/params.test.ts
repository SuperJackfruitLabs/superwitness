import { describe, expect, it } from "vitest";
import { match as id } from "./id";
import { match as rubricid } from "./rubricid";
import { match as version } from "./version";

describe("route param matchers", () => {
  it("id: letters, digits, underscore, hyphen, 1 to 128", () => {
    expect(id("brd_01")).toBe(true);
    expect(id("run-01")).toBe(true);
    expect(id("a".repeat(128))).toBe(true);
    expect(id("a".repeat(129))).toBe(false);
    expect(id("")).toBe(false);
    expect(id("brd 1")).toBe(false);
    expect(id("a.b")).toBe(false);
    expect(id("a/b")).toBe(false);
  });
  it("rubricid: as id, plus a dot, 1 to 64", () => {
    expect(rubricid("press")).toBe(true);
    expect(rubricid("release.v2_x-1")).toBe(true);
    expect(rubricid("a".repeat(64))).toBe(true);
    expect(rubricid("a".repeat(65))).toBe(false);
    expect(rubricid("")).toBe(false);
    expect(rubricid("a b")).toBe(false);
  });
  it("version: a positive integer of at most nine digits", () => {
    expect(version("1")).toBe(true);
    expect(version("2")).toBe(true);
    expect(version("123456789")).toBe(true);
    expect(version("1234567890")).toBe(false);
    expect(version("0")).toBe(false);
    expect(version("01")).toBe(false);
    expect(version("1a")).toBe(false);
    expect(version("")).toBe(false);
  });
});
