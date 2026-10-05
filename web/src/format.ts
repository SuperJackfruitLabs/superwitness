import type { RegistryRun } from "./types";

export type Tone = "good" | "warn" | "bad" | "info" | "quiet";

export function display(v: unknown): string {
  if (v === null || v === undefined) return "—";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

// A list the API may send as null or omit: never throws, never invents entries.
export function list<T>(v: T[] | null | undefined): T[] {
  return Array.isArray(v) ? v : [];
}

// A source status: ok, unavailable, timeout, not_found or unauthorized.
export function statusTone(s: string): Tone {
  if (s === "ok") return "good";
  if (s === "not_found" || s === "timeout") return "warn";
  return "bad";
}

// A run status from the registry.
export function runTone(s: string): Tone {
  switch (s) {
    case "succeeded":
      return "good";
    case "failed":
      return "bad";
    case "waiting":
      return "warn";
    case "running":
      return "info";
  }
  return "quiet";
}

export function duration(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

export function ago(iso: string, now: Date): string {
  const s = Math.round((now.getTime() - Date.parse(iso)) / 1000);
  if (s < 45) return "just now";
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} h ago`;
  return `${Math.round(h / 24)} d ago`;
}

// timing is a card's time: the duration of a finished run, else how long ago it started.
export function timing(run: RegistryRun, now: Date): string {
  if (run.started_at && run.ended_at) return duration(Date.parse(run.ended_at) - Date.parse(run.started_at));
  if (run.started_at) return `started ${ago(run.started_at, now)}`;
  return `first seen ${ago(run.first_seen_at, now)}`;
}

// verdictSummary says a verdict's value in a few words.
export function verdictSummary(value: Record<string, unknown> | null | undefined): string {
  if (!value) return "—";
  if (typeof value.decision === "string") return value.decision;
  if (typeof value.label === "string") return value.label;
  if (typeof value.score === "number") return value.score.toFixed(2);
  if (typeof value.text === "string") return value.text.length > 40 ? value.text.slice(0, 39) + "…" : value.text;
  return JSON.stringify(value);
}

// The helpers below serve the old token-paste run page only, and go when the app replaces it.

export const tokenKey = "superwitness.token";

export function parseRunPath(pathname: string): { board: string; run: string } | null {
  const m = /^\/runs\/superpipeline\/([A-Za-z0-9_-]+)\/([A-Za-z0-9_-]+)\/?$/.exec(pathname);
  return m ? { board: m[1], run: m[2] } : null;
}

export function emptyNote(sources: Record<string, string> | null | undefined, names: string[]): string {
  const bad = names.filter((n) => (sources ?? {})[n] !== "ok");
  if (bad.length === 0) return "none";
  return `unknown (${bad.map((n) => `source ${n}: ${display((sources ?? {})[n])}`).join(", ")})`;
}

export function isAuthStatus(status: number): boolean {
  return status === 401 || status === 403;
}
