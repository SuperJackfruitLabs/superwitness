import { describe, expect, it } from "vitest";
import { ago, display, duration, runTone, statusTone, timing, verdictSummary } from "./format";
import type { RegistryRun } from "./types";

describe("display", () => {
  it("never shows a missing value as zero or blank", () => {
    expect(display(null)).toBe("unknown");
    expect(display(undefined)).toBe("unknown");
    expect(display("unknown")).toBe("unknown");
    expect(display(0)).toBe("0");
  });
});

describe("tones", () => {
  it("maps source and run statuses", () => {
    expect(statusTone("ok")).toBe("good");
    expect(statusTone("timeout")).toBe("warn");
    expect(statusTone("unavailable")).toBe("bad");
    expect(runTone("failed")).toBe("bad");
    expect(runTone("waiting")).toBe("warn");
    expect(runTone("cancelled")).toBe("quiet");
  });
});

describe("time", () => {
  const now = new Date("2026-10-06T10:00:00Z");
  it("formats durations and ages", () => {
    expect(duration(4_000)).toBe("4s");
    expect(duration(296_000)).toBe("4m 56s");
    expect(duration(3_900_000)).toBe("1h 5m");
    expect(ago("2026-10-06T09:59:40Z", now)).toBe("just now");
    expect(ago("2026-10-06T09:57:00Z", now)).toBe("3 min ago");
    expect(ago("2026-10-06T07:00:00Z", now)).toBe("3 h ago");
    expect(ago("2026-10-01T10:00:00Z", now)).toBe("5 d ago");
  });
  it("times a card", () => {
    const run = { started_at: "2026-10-06T09:50:00Z", ended_at: "2026-10-06T09:54:56Z", first_seen_at: "2026-10-06T09:49:00Z" } as RegistryRun;
    expect(timing(run, now)).toBe("4m 56s");
    expect(timing({ ...run, ended_at: null }, now)).toBe("started 10 min ago");
    expect(timing({ ...run, ended_at: null, started_at: null }, now)).toBe("first seen 11 min ago");
  });
});

describe("verdictSummary", () => {
  it("says each kind of value", () => {
    expect(verdictSummary({ decision: "pass" })).toBe("pass");
    expect(verdictSummary({ score: 0.7 })).toBe("0.70");
    expect(verdictSummary({ label: "clear" })).toBe("clear");
    expect(verdictSummary({ text: "x".repeat(50) })).toHaveLength(40);
    expect(verdictSummary(null)).toBe("—");
  });
});
