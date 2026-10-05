import type { Health } from "./types";

// sourcesState is the sidebar dot: good when every source answers, else which ones do not.
export function sourcesState(h: Health | null, storeDown = false): { tone: "good" | "warn"; label: string } {
  if (storeDown) return { tone: "warn", label: "The database is not answering" };
  if (!h) return { tone: "warn", label: "Sources: unknown" };
  const down = Object.entries(h.sources ?? {}).filter(([, v]) => v !== "ok");
  if (down.length === 0) return { tone: "good", label: "All sources answer" };
  return { tone: "warn", label: down.map(([k, v]) => `${k}: ${v}`).join(", ") };
}
