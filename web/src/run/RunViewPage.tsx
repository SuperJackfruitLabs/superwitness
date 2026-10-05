import { useEffect, useState } from "react";
import { getJSON } from "../api";
import { ErrorView } from "../Edge";
import { list } from "../format";
import { useJSON } from "../hooks";
import { Link } from "../router";
import { StatusPill } from "../runs/RunCard";
import type { Doc, HistoryVerdict, Me } from "../types";
import { VerdictDrawer } from "../verdict/Drawer";
import { facts, TAB_LABEL, TAB_SOURCE, TABS, type Tab, tabOf } from "./facts";
import { AttemptsPanel, ErrorsPanel, LogsPanel, TracePanel, Unavailable, VerdictList } from "./Panels";

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

// useHistory reads every verdict on the run and on each of its attempts.
function useHistory(doc: Doc | null, reloadKey: number): HistoryVerdict[] {
  const [all, setAll] = useState<HistoryVerdict[]>([]);
  const ref: string | undefined = doc?.run?.ref;
  const attempts = list<Doc>(doc?.attempts).map((a) => a.id as string).filter(Boolean);
  const key = [ref, ...attempts].join(",");
  useEffect(() => {
    if (!ref) return;
    let live = true;
    const subjects = [["run", ref], ...attempts.map((a) => ["attempt", a])];
    Promise.all(
      subjects.map(([k, r]) =>
        getJSON<{ verdicts: HistoryVerdict[] }>(`/v1/verdicts?subject_kind=${k}&subject_ref=${encodeURIComponent(r)}`).then(
          (p) => p.verdicts,
          () => [] as HistoryVerdict[],
        ),
      ),
    ).then((vs) => live && setAll(vs.flat().sort((a, b) => a.created_at.localeCompare(b.created_at))));
    return () => {
      live = false;
    };
  }, [key, reloadKey]);
  return all;
}

export function RunViewPage({ board, run, query, me }: { board: string; run: string; query: string; me: Me }) {
  const base = `/runs/superpipeline/${board}/${run}`;
  const api = `/v1${base}`;
  const doc = useJSON<Doc>(api);
  const tab = tabOf(new URLSearchParams(query).get("tab"));
  const [evidence, setEvidence] = useState<string[]>([]);
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
    panel = <TracePanel base={api} evidence={evidence} setEvidence={setEvidence} />;
  } else if (tab === "logs") {
    panel = <LogsPanel base={api} />;
  } else if (tab === "errors") {
    panel = <ErrorsPanel doc={d} />;
  } else if (tab === "verdicts") {
    panel = (
      <VerdictList verdicts={history} gates={list<Doc>(d.verdicts).filter((v) => v.kind === "gate")} me={me} onRevise={(v) => setDrawer({ revising: v })} />
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
