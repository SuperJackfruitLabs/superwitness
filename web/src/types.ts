// The shapes the app reads from superwitness's API.

export type RunStatus = "queued" | "running" | "waiting" | "succeeded" | "failed" | "cancelled";
export const STATUSES: RunStatus[] = ["queued", "running", "waiting", "succeeded", "failed", "cancelled"];

export interface Named {
  id: string | null;
  name: string | null;
}

export interface VerdictSummary {
  id: string;
  judge: string;
  judge_kind: string;
  standard: string;
  value: Record<string, unknown>;
  created_at: string;
}

export interface RegistryRun {
  source: string;
  external_ref: string;
  ref: string;
  scope: Named | null;
  title: string | null;
  executor: Named | null;
  status: RunStatus;
  source_status: string;
  started_at: string | null;
  ended_at: string | null;
  reported_at: string;
  first_seen_at: string;
  updated_at: string;
  latest_verdict: VerdictSummary | null;
}

export interface RunPage {
  runs: RegistryRun[];
  next_cursor: string | null;
  counts: Record<RunStatus, number>;
}

export interface Me {
  principal: string;
  kind: string;
  email: string | null;
  via: "session" | "bearer";
}

export interface Scale {
  kind: "decision" | "score" | "label" | "text";
  options?: string[];
  labels?: string[];
  min?: number;
  max?: number;
}

export interface Rubric {
  id: string;
  version: number;
  standard: string;
  name: string;
  scale: unknown;
  recognised_scale: Scale | null;
  body?: string;
  created_by: string;
  created_at: string;
}

export interface HistoryVerdict {
  id: string;
  kind: string;
  subject_kind: string;
  subject_ref: string;
  judge: string;
  judge_kind: string;
  standard: string;
  value: Record<string, unknown>;
  comment: string;
  evidence_refs: unknown[];
  supersedes: string | null;
  superseded_by: string | null;
  created_at: string;
}

export interface Span {
  trace_id: string;
  span_id: string;
  parent_span_id?: string;
  name: string;
  service: string;
  start: string;
  duration_ms: number;
  attributes: Record<string, string>;
}

export interface LogLine {
  at: string;
  service: string;
  level: string;
  message: string;
  trace_id?: string;
  span_id?: string;
}

export interface ScopeEntry {
  source: string;
  id: string;
  name: string | null;
  runs: number;
}

export interface Health {
  status: string;
  version: string;
  sources: Record<string, string>;
}

// The run document (internal/join/document.go), read loosely: the page shows what it gets.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type Doc = Record<string, any>;
