import { describe, expect, it } from "vitest";
import type { LogLine, Span } from "../types";
import {
  hasTruncated, itemFirstSeq, itemRange, itemURL, logsOf, parseSeq, rangeRef, requestResponse, runURL, sameRef, seqParam, showValue,
  spanFor, spanRange, spanStatus, type TranscriptItem, transcriptURL,
} from "./transcript";

const API = "/v1/runs/superpipeline/brd_01/run_01";
const PAGE = "/runs/superpipeline/brd_01/run_01";
const attempts = [{ id: "attempt_a1", session_id: "acps_a1", seq_from: 1, seq_to: 9 }, { id: "attempt_a2", session_id: "acps_a2", seq_from: 10, seq_to: null }];
const span = (id: string, name: string, attributes: Record<string, string>, start = "2026-10-04T10:00:00Z", duration_ms = 1000): Span =>
  ({ trace_id: "t1", span_id: id, name, service: "hub", start, duration_ms, attributes });

describe("spanRange", () => {
  it("reads a content span's seqs within its attempt", () => {
    expect(spanRange(span("s1", "tool_call", { "attempt.id": "attempt_a1", "acp.seq_from": "3", "acp.seq_to": "4" }), attempts))
      .toEqual({ attempt: "attempt_a1", session: "acps_a1", from: 3, to: 4 });
  });
  it("falls back to the attempt's seqs where the span has none", () => {
    expect(spanRange(span("s1", "attempt", { "attempt.id": "attempt_a1" }), attempts)).toEqual({ attempt: "attempt_a1", session: "acps_a1", from: 1, to: 9 });
    expect(spanRange(span("s2", "turn", { "attempt.id": "attempt_a2", "acp.seq_from": "10" }), attempts)).toEqual({ attempt: "attempt_a2", session: "acps_a2", from: 10, to: null });
  });
  it("has none for other spans, unknown attempts and garbled seqs", () => {
    expect(spanRange(span("s1", "GET /x", { "attempt.id": "attempt_a1", "acp.seq_from": "3" }), attempts)).toBeNull();
    expect(spanRange(span("s1", "turn", { "attempt.id": "attempt_zz", "acp.seq_from": "3" }), attempts)).toBeNull();
    expect(spanRange(span("s1", "turn", { "attempt.id": "attempt_a1", "acp.seq_from": "three" }), attempts)).toBeNull();
    expect(spanRange(span("s1", "turn", { "attempt.id": "attempt_a1", "acp.seq_from": "3" }), [{ id: "attempt_a1", session_id: "unknown", seq_from: 1, seq_to: 9 }])).toBeNull();
  });
});

describe("spanFor", () => {
  const spans = [
    span("sp_attempt", "attempt", { "attempt.id": "attempt_a1", "acp.seq_from": "1", "acp.seq_to": "9" }),
    span("sp_turn", "turn", { "attempt.id": "attempt_a1", "acp.seq_from": "1", "acp.seq_to": "9" }),
    span("sp_tool", "tool_call", { "attempt.id": "attempt_a1", "acp.seq_from": "3", "acp.seq_to": "4" }),
    span("sp_http", "GET /x", {}),
  ];
  it("picks the narrowest content span that holds the card", () => {
    expect(spanFor(spans, attempts, "attempt_a1", 3, 4)?.span_id).toBe("sp_tool");
    expect(spanFor(spans, attempts, "attempt_a1", 1, 1)?.span_id).toBe("sp_attempt");
    expect(spanFor(spans, attempts, "attempt_a2", 10, 10)).toBeNull();
  });
});

describe("items", () => {
  it("knows each kind's range", () => {
    expect(itemRange({ kind: "prompt", seq: 1 })).toEqual({ from: 1, to: 1 });
    expect(itemRange({ kind: "permission", seq: 5, answer_seq: 6 })).toEqual({ from: 5, to: 6 });
    expect(itemRange({ kind: "tool_call", seq_from: 3, seq_to: 4 })).toEqual({ from: 3, to: 4 });
  });
  it("names an item by the seq the hub addresses it with", () => {
    expect(itemFirstSeq({ kind: "tool_call", seq_from: 3, seq_to: 4 })).toBe(3);
    expect(itemFirstSeq({ kind: "permission", seq: 5, answer_seq: 6 })).toBe(5);
    expect(itemFirstSeq({ kind: "permission", seq: 5, answer_seq: 6, partial: false })).toBe(5);
    // a partial permission's request lies before the range it was shown in: the hub names it by its answer
    expect(itemFirstSeq({ kind: "permission", seq: 5, answer_seq: 6, partial: true })).toBe(6);
    expect(itemFirstSeq({ kind: "permission", seq: 5, partial: true })).toBe(5);
  });
  it("shows a cut field's head and size, at any depth", () => {
    const cut = { truncated: true as const, bytes: 40000, head: "## 0.4" };
    expect(showValue(cut)).toBe("## 0.4\u2026 [40000 bytes, cut]");
    expect(hasTruncated({ content: [{ text: cut }] })).toBe(true);
    expect(hasTruncated({ content: [{ text: "x" }] })).toBe(false);
    expect(showValue({ content: [{ text: cut }] })).toContain("## 0.4\u2026 [40000 bytes, cut]");
    expect(showValue("plain")).toBe("plain");
  });
  it("finds a span's request and response", () => {
    const items: TranscriptItem[] = [
      { kind: "prompt", seq: 1, text: "ask" },
      { kind: "tool_call", id: "tc", seq_from: 3, seq_to: 4, input: { path: "a" }, output: { content: "b" } },
      { kind: "permission", seq: 5, answer_seq: 6, options: [{ name: "Allow" }], outcome: "selected:allow" },
      { kind: "message", seq_from: 9, seq_to: 9, text: "done" },
    ];
    const tool = requestResponse("tool_call", items, 3);
    expect(tool.request.map((p) => p.value)).toEqual([{ path: "a" }]);
    expect(tool.response.map((p) => p.value)).toEqual([{ content: "b" }]);
    const perm = requestResponse("permission", items, 5);
    expect(perm.response.map((p) => p.value)).toEqual(["selected:allow"]);
    const turn = requestResponse("turn", items, 1);
    expect(turn.request.map((p) => p.value)).toEqual(["ask"]);
    expect(turn.response.map((p) => p.value)).toEqual(["done"]);
  });
});

describe("URLs", () => {
  it("keeps tab, span, attempt and seq, and leaves the default tab out", () => {
    expect(runURL(PAGE, { span: "sp_tool" })).toBe(`${PAGE}?span=sp_tool`);
    expect(runURL(PAGE, { tab: "trace", span: "sp_tool" })).toBe(`${PAGE}?span=sp_tool`);
    expect(runURL(PAGE, { tab: "transcript", attempt: "attempt_a1", seq: "3-4" })).toBe(`${PAGE}?tab=transcript&attempt=attempt_a1&seq=3-4`);
    expect(runURL(PAGE, {})).toBe(PAGE);
  });
  it("writes and reads a seq", () => {
    expect(seqParam(3, 4)).toBe("3-4");
    expect(seqParam(3, 3)).toBe("3");
    expect(seqParam(3, null)).toBe("3");
    expect(parseSeq("3-4")).toEqual({ from: 3, to: 4 });
    expect(parseSeq("7")).toEqual({ from: 7, to: 7 });
    expect(parseSeq("4-3")).toBeNull();
    expect(parseSeq("x")).toBeNull();
    expect(parseSeq(null)).toBeNull();
  });
  it("names the attempt on every transcript read", () => {
    expect(transcriptURL(API, "attempt_a1", { from: 3, to: 4 })).toBe(`${API}/transcript?attempt=attempt_a1&seq_from=3&seq_to=4`);
    expect(transcriptURL(API, "attempt_a2", { from: 10, to: null })).toBe(`${API}/transcript?attempt=attempt_a2&seq_from=10`);
    expect(transcriptURL(API, "attempt_a1", { cursor: "c 2" })).toBe(`${API}/transcript?attempt=attempt_a1&cursor=c+2`);
    expect(itemURL(API, "attempt_a1", 3)).toBe(`${API}/transcript/items/3?attempt=attempt_a1&full=1`);
  });
  it("forwards the range an item was shown in when asking for it in full", () => {
    expect(itemURL(API, "attempt_a1", 3, { from: 3, to: 4 })).toBe(`${API}/transcript/items/3?attempt=attempt_a1&seq_from=3&seq_to=4&full=1`);
    expect(itemURL(API, "attempt_a2", 10, { from: 10, to: null })).toBe(`${API}/transcript/items/10?attempt=attempt_a2&seq_from=10&full=1`);
  });
});

describe("spans' details", () => {
  it("reads a status from the attributes", () => {
    expect(spanStatus(span("s", "x", { "otel.status_code": "ERROR" }))).toBe("ERROR");
    expect(spanStatus(span("s", "x", { error: "true" }))).toBe("ERROR");
    expect(spanStatus(span("s", "x", {}))).toBe("unset");
  });
  it("keeps the log lines in the span's window and trace", () => {
    const s = span("s", "tool_call", {}, "2026-10-04T10:00:10Z", 2000);
    const line = (at: string, trace_id?: string): LogLine => ({ at, service: "hub", level: "info", message: at, trace_id });
    const kept = logsOf([line("2026-10-04T10:00:09Z", "t1"), line("2026-10-04T10:00:11Z", "t1"), line("2026-10-04T10:00:11Z", "t9"),
      line("2026-10-04T10:00:11.5Z"), line("2026-10-04T10:00:13Z", "t1")], s);
    expect(kept.map((l) => l.at)).toEqual(["2026-10-04T10:00:11Z", "2026-10-04T10:00:11.5Z"]);
  });
});

describe("evidence refs", () => {
  it("compares a range by value", () => {
    expect(sameRef(rangeRef("acps_a1", 3, 4), { session_id: "acps_a1", seq_from: 3, seq_to: 4 })).toBe(true);
    expect(sameRef(rangeRef("acps_a1", 3, 4), rangeRef("acps_a1", 3, 5))).toBe(false);
    expect(sameRef("aaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaa")).toBe(true);
  });
});
