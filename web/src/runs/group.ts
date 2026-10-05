import type { RegistryRun } from "../types";

// A run's moment on the timeline: when it started, or when superwitness first heard of it.
export const runTime = (r: RegistryRun) => r.started_at ?? r.first_seen_at;

// dayKey is the calendar day of iso in timeZone (the viewer's own when undefined), as YYYY-MM-DD.
export function dayKey(iso: string, timeZone?: string): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone, year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date(iso));
}

export function dayLabel(key: string, now: Date, timeZone?: string): string {
  const today = dayKey(now.toISOString(), timeZone);
  if (key === today) return "Today";
  const [y, m, d] = today.split("-").map(Number);
  if (key === new Date(Date.UTC(y, m - 1, d - 1)).toISOString().slice(0, 10)) return "Yesterday";
  const [ky, km, kd] = key.split("-").map(Number);
  return new Intl.DateTimeFormat("en-GB", { timeZone: "UTC", weekday: "short", day: "numeric", month: "short", year: "numeric" }).format(
    new Date(Date.UTC(ky, km - 1, kd)),
  );
}

export interface DayGroup {
  key: string;
  label: string;
  runs: RegistryRun[];
}

// groupByDay keeps the API's order (newest first) and starts a group at each new day.
export function groupByDay(runs: RegistryRun[], now: Date, timeZone?: string): DayGroup[] {
  const out: DayGroup[] = [];
  for (const r of runs) {
    const key = dayKey(runTime(r), timeZone);
    const last = out[out.length - 1];
    if (last && last.key === key) last.runs.push(r);
    else out.push({ key, label: dayLabel(key, now, timeZone), runs: [r] });
  }
  return out;
}
