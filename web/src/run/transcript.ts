// Session content as superwitness's transcript routes return it, and the small rules the span pane
// and the Transcript tab share. Nothing here fetches.
import type { Doc, EvidenceRef, LogLine, Span } from "../types";

// A string field the hub cut at 16 KiB.
export interface Truncated {
  truncated: true;
  bytes: number;
  head: string;
}

export interface TranscriptItem {
  kind: string; // prompt | message | reasoning | tool_call | permission | state | error | other
  seq?: number;
  seq_from?: number;
  seq_to?: number;
  answer_seq?: number;
  text?: string | Truncated;
  images?: { name: string; mimeType: string }[];
  id?: string;
  title?: string | Truncated;
  tool_kind?: string;
  status?: string;
  input?: unknown;
  output?: unknown;
  partial?: boolean;
  tool_call_id?: string;
  options?: unknown;
  outcome?: string;
  reason?: string;
  error_kind?: string;
  message?: string | Truncated;
  type?: string;
  redactions?: number;
}

export interface TranscriptPage {
  attempt_id: string;
  session_id: string;
  seq_from: number;
  seq_to: number;
  items: TranscriptItem[];
  next_cursor: string | null;
  redactions: number;
  truncated_fields: number;
}

export interface SeqRange {
  attempt: string;
  session: string;
  from: number;
  to: number | null; // null: the attempt is still running
}

export interface Part {
  item: TranscriptItem;
  value: unknown;
}

// The spans whose seqs map to the agent session.
export const CONTENT_SPANS = ["attempt", "turn", "tool_call", "permission"];

export const isTruncated = (v: unknown): v is Truncated =>
  typeof v === "object" && v !== null && (v as Truncated).truncated === true && typeof (v as Truncated).head === "string";

export function hasTruncated(v: unknown): boolean {
  if (isTruncated(v)) return true;
  if (Array.isArray(v)) return v.some(hasTruncated);
  if (v && typeof v === "object") return Object.values(v).some(hasTruncated);
  return false;
}

const cutText = (t: Truncated) => `${t.head}… [${t.bytes} bytes, cut]`;

// showValue is a field as text: a string as it is, anything else as indented JSON, a cut field as
// its head and size.
export function showValue(v: unknown): string {
  if (typeof v === "string") return v;
  if (isTruncated(v)) return cutText(v);
  return JSON.stringify(v, (_k, x) => (isTruncated(x) ? cutText(x) : x), 2) ?? "none";
}

export function itemRange(it: TranscriptItem): { from: number; to: number } {
  const from = it.seq_from ?? it.seq ?? 0;
  return { from, to: it.seq_to ?? it.answer_seq ?? it.seq ?? from };
}

// itemFirstSeq is the seq the hub addresses an item by. A permission cut by the range has its
// request before the range, so it is named by its answer.
export function itemFirstSeq(it: TranscriptItem): number {
  if (it.kind === "permission" && it.partial && it.answer_seq !== undefined) return it.answer_seq;
  return itemRange(it).from;
}

export const holds = (r: { from: number; to: number }, seq: number) => seq >= r.from && seq <= r.to;

const whole = (v: string | undefined) => (v !== undefined && /^\d+$/.test(v) ? Number(v) : null);

// spanRange is the session range a span maps to: its acp.seq_from..acp.seq_to within its attempt.
// An attempt span without seqs uses its attempt's; a span not yet ended runs to its attempt's end.
export function spanRange(span: Span, attempts: Doc[]): SeqRange | null {
  if (!CONTENT_SPANS.includes(span.name)) return null;
  const a = attempts.find((x) => x.id === span.attributes["attempt.id"]);
  if (!a || typeof a.session_id !== "string" || a.session_id === "" || a.session_id === "unknown") return null;
  const raw = span.attributes["acp.seq_from"];
  const from = raw === undefined && span.name === "attempt" ? a.seq_from : whole(raw);
  if (typeof from !== "number") return null;
  const to = whole(span.attributes["acp.seq_to"]) ?? (typeof a.seq_to === "number" ? a.seq_to : null);
  return { attempt: a.id, session: a.session_id, from, to };
}

// spanFor is the span a card links to: of the content spans in the attempt whose range holds the
// card's, the narrowest.
export function spanFor(spans: Span[], attempts: Doc[], attempt: string, from: number, to: number): Span | null {
  let best: Span | null = null;
  let width = Infinity;
  for (const s of spans) {
    const r = spanRange(s, attempts);
    if (!r || r.attempt !== attempt || r.from > from || (r.to !== null && r.to < to)) continue;
    const w = (r.to ?? Number.MAX_SAFE_INTEGER) - r.from;
    if (w < width) {
      best = s;
      width = w;
    }
  }
  return best;
}

export function spanStatus(span: Span): string {
  return span.attributes["otel.status_code"] ?? (span.attributes.error === "true" ? "ERROR" : "unset");
}

// logsOf keeps the lines inside the span's time window that belong to its trace or name none.
export function logsOf(lines: LogLine[], span: Span): LogLine[] {
  const t0 = Date.parse(span.start);
  const t1 = t0 + span.duration_ms;
  return lines.filter((l) => {
    const t = Date.parse(l.at);
    return (!l.trace_id || l.trace_id === span.trace_id) && t >= t0 && t <= t1;
  });
}

// requestResponse is what the pane shows for a span: a tool call's input and output, a
// permission's options and outcome, else the prompts and the agent's messages.
export function requestResponse(spanName: string, items: TranscriptItem[], from: number): { request: Part[]; response: Part[] } {
  if (spanName === "tool_call") {
    const it = items.find((i) => i.kind === "tool_call" && itemRange(i).from === from) ?? items.find((i) => i.kind === "tool_call");
    return it ? { request: [{ item: it, value: it.input }], response: [{ item: it, value: it.output }] } : { request: [], response: [] };
  }
  if (spanName === "permission") {
    const it = items.find((i) => i.kind === "permission" && i.seq === from) ?? items.find((i) => i.kind === "permission");
    return it ? { request: [{ item: it, value: it.options }], response: [{ item: it, value: it.outcome }] } : { request: [], response: [] };
  }
  return {
    request: items.filter((i) => i.kind === "prompt").map((i) => ({ item: i, value: i.text })),
    response: items.filter((i) => i.kind === "message").map((i) => ({ item: i, value: i.text })),
  };
}

// runURL is a run page's address with its state: the tab (Trace, the default, is left out), the
// selected span, and the Transcript's attempt and seq.
export function runURL(page: string, p: { tab?: string; span?: string; attempt?: string; seq?: string }): string {
  const q = new URLSearchParams();
  if (p.tab && p.tab !== "trace") q.set("tab", p.tab);
  if (p.span) q.set("span", p.span);
  if (p.attempt) q.set("attempt", p.attempt);
  if (p.seq) q.set("seq", p.seq);
  const s = q.toString();
  return s ? `${page}?${s}` : page;
}

export const seqParam = (from: number, to: number | null) => (to === null || to === from ? String(from) : `${from}-${to}`);

export function parseSeq(v: string | null): { from: number; to: number } | null {
  const m = v ? /^(\d+)(?:-(\d+))?$/.exec(v) : null;
  if (!m) return null;
  const from = Number(m[1]);
  const to = m[2] === undefined ? from : Number(m[2]);
  return to >= from ? { from, to } : null;
}

export function transcriptURL(api: string, attempt: string, o: { from?: number; to?: number | null; cursor?: string | null } = {}): string {
  const q = new URLSearchParams({ attempt });
  if (o.from !== undefined) q.set("seq_from", String(o.from));
  if (o.to !== undefined && o.to !== null) q.set("seq_to", String(o.to));
  if (o.cursor) q.set("cursor", o.cursor);
  return `${api}/transcript?${q}`;
}

// itemURL asks for one item in full. The range it was shown in rides along so the hub resolves the
// same item the page lists; the hub names an item by its first seq.
export function itemURL(api: string, attempt: string, seqFrom: number, range?: { from: number; to: number | null }): string {
  const q = new URLSearchParams({ attempt });
  if (range) {
    q.set("seq_from", String(range.from));
    if (range.to !== null) q.set("seq_to", String(range.to));
  }
  q.set("full", "1");
  return `${api}/transcript/items/${seqFrom}?${q}`;
}

// rangeRef is a cited step, its keys in the order the verdict API documents.
export const rangeRef = (session: string, from: number, to: number): EvidenceRef => ({ session_id: session, seq_from: from, seq_to: to });

export const sameRef = (a: EvidenceRef, b: EvidenceRef) => JSON.stringify(a) === JSON.stringify(b);
