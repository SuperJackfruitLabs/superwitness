// The runs view's filters live in the URL, so a view can be bookmarked. They are GET /v1/runs's
// own parameters.

const KEYS = ["source", "scope", "status", "executor", "since", "until", "needs_verdict"] as const;

export function apiQuery(search: URLSearchParams): string {
  const out = new URLSearchParams();
  for (const k of KEYS) for (const v of search.getAll(k)) out.append(k, v);
  return out.toString();
}

export function hasFilters(search: URLSearchParams): boolean {
  return KEYS.some((k) => search.has(k));
}

export function viewTitle(search: URLSearchParams): string {
  const statuses = search.getAll("status");
  if (search.get("needs_verdict") === "true") return "Needs verdict";
  if (search.get("scope")) return `Board ${search.get("scope")}`;
  if (statuses.length === 1 && statuses[0] === "failed" && !hasOther(search, "status")) return "Failed";
  if (statuses.length === 1 && statuses[0] === "waiting" && !hasOther(search, "status")) return "Waiting";
  return hasFilters(search) ? "Runs" : "All runs";
}

function hasOther(search: URLSearchParams, key: string): boolean {
  return KEYS.some((k) => k !== key && search.has(k));
}

// withStatus is the view with only this status, or with no status when it is already the one.
export function withStatus(search: URLSearchParams, status: string): string {
  const next = new URLSearchParams(apiQuery(search));
  const only = next.getAll("status");
  next.delete("status");
  if (!(only.length === 1 && only[0] === status)) next.append("status", status);
  const q = next.toString();
  return q ? `/?${q}` : "/";
}
