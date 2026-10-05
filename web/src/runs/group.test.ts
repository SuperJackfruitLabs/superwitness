import { describe, expect, it } from "vitest";
import type { RegistryRun } from "../types";
import { dayKey, dayLabel, groupByDay } from "./group";

const run = (ref: string, started: string | null, firstSeen = "2026-10-01T00:00:00Z") =>
  ({ external_ref: ref, started_at: started, first_seen_at: firstSeen }) as RegistryRun;

describe("day grouping", () => {
  // 04:00 UTC on 6 October: 09:30 in Kolkata, still 00:00 in New York.
  const now = new Date("2026-10-06T04:00:00Z");

  it("puts a run in the viewer's own day, across midnight", () => {
    const lateUTC = "2026-10-05T19:00:00Z"; // 00:30 on the 6th in Kolkata, 15:00 on the 5th in New York
    expect(dayKey(lateUTC, "Asia/Kolkata")).toBe("2026-10-06");
    expect(dayKey(lateUTC, "America/New_York")).toBe("2026-10-05");
    expect(groupByDay([run("a", lateUTC)], now, "Asia/Kolkata")[0].label).toBe("Today");
    expect(groupByDay([run("a", lateUTC)], now, "America/New_York")[0].label).toBe("Yesterday");
  });

  it("labels older days with their date", () => {
    expect(dayLabel("2026-10-01", now, "UTC")).toContain("1 Oct 2026");
  });

  it("keeps the API's order and starts a group at each new day", () => {
    const g = groupByDay(
      [run("a", "2026-10-06T03:00:00Z"), run("b", "2026-10-06T01:00:00Z"), run("c", "2026-10-05T23:00:00Z"), run("d", null, "2026-10-04T12:00:00Z")],
      now,
      "UTC",
    );
    expect(g.map((x) => [x.label, x.runs.map((r) => r.external_ref).join("")])).toEqual([
      ["Today", "ab"],
      ["Yesterday", "c"],
      [dayLabel("2026-10-04", now, "UTC"), "d"],
    ]);
  });
});
