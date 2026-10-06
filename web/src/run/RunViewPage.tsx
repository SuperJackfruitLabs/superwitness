import { useEffect, useState } from "react";
import { type ApiError, getJSON } from "../api";
import { ErrorView } from "../Edge";
import { list } from "../format";
import { useJSON } from "../hooks";
import { Link } from "../router";
import { StatusPill } from "../runs/RunCard";
import type { Doc, EvidenceRef, HistoryVerdict, Me } from "../types";
import { VerdictDrawer } from "../verdict/Drawer";
import { facts, TAB_LABEL, TAB_SOURCE, TABS, type Tab, tabOf } from "./facts";
import { AttemptsPanel, ErrorsPanel, type Failure, LogsPanel, TracePanel, Unavailable, VerdictList } from "./Panels";

export function Facts({ doc }: { doc: Doc }) {
  return (
    <dl className="facts">
      {facts(doc).map((f) => (
        <div key={f.label}>
          <dt>{f.label}</dt>
          <dd>{f.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function Tabs({ current, base }: { current: Tab; base: string }) {
  return (
    <nav className="tabs" role="tablist" aria-label="Run">
      {TABS.map((t) => (
        <Link key={t} to={t === "trace" ? base : `${base}?tab=${t}`} role="tab" aria-selected={t === current}>
          {TAB_LABEL[t]}
        </Link>
      ))}
    </nav>
  );
}

interface History {
  verdicts: HistoryVerdict[];
  failures: Failure[];
  loading: boolean;
  retry: () => void;
}

// useHistory reads every verdict on the run and on each of its attempts. A subject whose read failed
// is reported in failures; it is never folded into an empty list.
function useHistory(doc: Doc | null, reloadKey: number): History {
  const [state, setState] = useState<{ verdicts: HistoryVerdict[]; failures: Failure[]; loading: boolean }>({ verdicts: [], failures: [], loading: true });
  const [tries, setTries] = useState(0);
  const ref: string | undefined = doc?.run?.ref;
  const attempts = list<Doc>(doc?.attempts).map((a) => a.id as string).filter(Boolean);
  const key = [ref, ...attempts].join(",");
  useEffect(() => {
    if (!ref) return;
    let live = true;
    setState((s) => ({ ...s, loading: true }));
    const subjects = [["run", ref], ...attempts.map((a) => ["attempt", a])];
    Promise.all(
      subjects.map(([k, r]) =>
        getJSON<{ verdicts: HistoryVerdict[] }>(`/v1/verdicts?subject_kind=${k}&subject_ref=${encodeURIComponent(r)}`).then(
          (p) => ({ ok: list(p.verdicts) }),
          (e: ApiError) => ({ fail: { subject: k === "run" ? "this run" : `attempt ${r}`, message: e.message } }),
        ),
      ),
    ).then((rs) => {
      if (!live) return;
      const verdicts: HistoryVerdict[] = [];
      const failures: Failure[] = [];
      for (const r of rs) {
        if ("ok" in r) verdicts.push(...r.ok);
        else failures.push(r.fail);
      }
      verdicts.sort((a, b) => a.created_at.localeCompare(b.created_at));
      setState({ verdicts, failures, loading: false });
    });
    return () => {
      live = false;
    };
  }, [key, reloadKey, tries]);
  return { ...state, retry: () => setTries((n) => n + 1) };
}

// A run view belongs to one run: keyed by it, so nothing ticked or typed on one run (evidence, an
// open drawer) can reach another.
export function RunViewPage(props: { board: string; run: string; query: string; me: Me }) {
  return <RunView key={`${props.board}/${props.run}`} {...props} />;
}

function RunView({ board, run, query, me }: { board: string; run: string; query: string; me: Me }) {
  const base = `/runs/superpipeline/${board}/${run}`;
  const api = `/v1${base}`;
  const doc = useJSON<Doc>(api);
  const params = new URLSearchParams(query);
  const tab = tabOf(params.get("tab"));
  const selectedSpan = params.get("span");
  const [evidence, setEvidence] = useState<EvidenceRef[]>([]);
  const [drawer, setDrawer] = useState<{ revising?: HistoryVerdict } | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const history = useHistory(doc.data, reloadKey);

  if (doc.error) return <ErrorView error={doc.error} />;
  if (!doc.data) return <p className="loading">Loading…</p>;
  const d = doc.data;
  const sources: Record<string, string> = d.sources ?? {};
  const source = TAB_SOURCE[tab];
  const title = d.run?.card?.title && d.run.card.title !== "unknown" ? d.run.card.title : `${board}/${run}`;

  let panel;
  if (sources[source] !== "ok") {
    panel = <Unavailable source={source} status={sources[source]} />;
  } else if (tab === "trace") {
    panel = <TracePanel base={api} page={base} attempts={list(d.attempts)} selected={selectedSpan} evidence={evidence} setEvidence={setEvidence} />;
  } else if (tab === "logs") {
    panel = <LogsPanel base={api} />;
  } else if (tab === "errors") {
    panel = <ErrorsPanel doc={d} />;
  } else if (tab === "verdicts") {
    panel = (
      <VerdictList attempts={list(d.attempts)} page={base} verdicts={history.verdicts} failures={history.failures} loading={history.loading} onRetry={history.retry} gates={list<Doc>(d.verdicts).filter((v) => v.kind === "gate")} me={me} onRevise={(v) => setDrawer({ revising: v })} />
    );
  } else {
    panel = <AttemptsPanel doc={d} />;
  }

  return (
    <article>
      <header>
        <p className="mono ref">
          superpipeline · {board}/{run}
        </p>
        <h1>{title}</h1>
        <div className="headrow">
          <StatusPill status={d.run?.state ?? "unknown"} />
          <button className="primary" onClick={() => setDrawer({})}>
            Record verdict
          </button>
        </div>
        <Facts doc={d} />
      </header>
      <Tabs current={tab} base={base} />
      <div role="tabpanel" aria-label={TAB_LABEL[tab]}>
        {panel}
      </div>
      {drawer && (
        <VerdictDrawer
          doc={d}
          evidence={evidence}
          revising={drawer.revising}
          onClose={() => setDrawer(null)}
          onRecorded={() => {
            setDrawer(null);
            setReloadKey((k) => k + 1);
            doc.reload();
          }}
        />
      )}
    </article>
  );
}
