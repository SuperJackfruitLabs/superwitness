export type Tone = "good" | "warn" | "bad";

export const tokenKey = "superwitness.token";

export function parseRunPath(pathname: string): { board: string; run: string } | null {
  const m = /^\/runs\/superpipeline\/([A-Za-z0-9_-]+)\/([A-Za-z0-9_-]+)\/?$/.exec(pathname);
  return m ? { board: m[1], run: m[2] } : null;
}

export function display(v: unknown): string {
  if (v === null || v === undefined) return "—";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

export function statusTone(s: string): Tone {
  if (s === "ok") return "good";
  if (s === "not_found" || s === "timeout") return "warn";
  return "bad";
}

// A list the API may send as null or omit: never throws, never invents entries.
export function list<T>(v: T[] | null | undefined): T[] {
  return Array.isArray(v) ? v : [];
}

// Text for an empty list. "none" only when every backing source answered ok; otherwise the
// emptiness is not evidence of absence, so say unknown and why.
export function emptyNote(sources: Record<string, string> | null | undefined, names: string[]): string {
  const bad = names.filter((n) => (sources ?? {})[n] !== "ok");
  if (bad.length === 0) return "none";
  return `unknown (${bad.map((n) => `source ${n}: ${display((sources ?? {})[n])}`).join(", ")})`;
}

export function isAuthStatus(status: number): boolean {
  return status === 401 || status === 403;
}
