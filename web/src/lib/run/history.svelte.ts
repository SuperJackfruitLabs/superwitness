import { type ApiError, getJSON } from "../api";
import { list } from "../format";
import type { Doc, HistoryVerdict } from "../types";

export interface Failure {
  subject: string;
  message: string;
}

export interface History {
  readonly verdicts: HistoryVerdict[];
  readonly failures: Failure[];
  readonly loading: boolean;
  retry(): void;
}

// history reads every verdict on the run and on each of its attempts (the old useHistory), oldest
// first. A subject whose read failed is reported in failures; it is never folded into an empty list.
// A read that answers after the run document changed, or after a retry, is dropped. Call it while a
// component initialises: it creates an effect.
export function history(doc: () => Doc | null): History {
  let verdicts = $state.raw<HistoryVerdict[]>([]);
  let failures = $state.raw<Failure[]>([]);
  let loading = $state(true);
  let tries = $state(0);
  const key = $derived.by(() => {
    const d = doc();
    const ref: string | undefined = d?.run?.ref;
    if (!ref) return null;
    return JSON.stringify([ref, ...list<Doc>(d?.attempts).map((a) => a.id as string).filter(Boolean)]);
  });
  $effect(() => {
    void tries;
    if (!key) return;
    const [ref, ...attempts] = JSON.parse(key) as string[];
    let live = true;
    loading = true;
    const subjects = [["run", ref], ...attempts.map((a) => ["attempt", a])];
    Promise.all(
      subjects.map(([k, r]) =>
        getJSON<{ verdicts: HistoryVerdict[] }>(`/v1/verdicts?subject_kind=${k}&subject_ref=${encodeURIComponent(r)}`).then(
          (p) => ({ ok: list<HistoryVerdict>(p.verdicts) }),
          (e: ApiError) => ({ fail: { subject: k === "run" ? "this run" : `attempt ${r}`, message: e.message } }),
        ),
      ),
    ).then((rs) => {
      if (!live) return;
      const vs: HistoryVerdict[] = [];
      const fs: Failure[] = [];
      for (const r of rs) {
        if ("ok" in r) vs.push(...r.ok);
        else fs.push(r.fail);
      }
      vs.sort((a, b) => a.created_at.localeCompare(b.created_at));
      verdicts = vs;
      failures = fs;
      loading = false;
    });
    return () => {
      live = false;
    };
  });
  return {
    get verdicts() {
      return verdicts;
    },
    get failures() {
      return failures;
    },
    get loading() {
      return loading;
    },
    retry() {
      tries++;
    },
  };
}
