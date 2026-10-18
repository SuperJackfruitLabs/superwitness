import { useEffect, useRef, useState } from "react";
import { ApiError, getJSON } from "../api";
import { display, list } from "../format";
import type { Doc, EvidenceRef, HistoryVerdict, LogLine, Me, Span } from "../types";
import { verdictSummary } from "../format";
import { MAX_EVIDENCE } from "../verdict/Drawer";
import { Link, navigate } from "../router";
import { waterfall } from "./facts";
import { SpanPane } from "./SpanPane";
import { runURL, seqParam } from "./transcript";

export { MAX_EVIDENCE };

export function Unavailable({ source, status }: { source: string; status: string | undefined }) {
  return (
    <p className="unavailable">
      unavailable: the {source} source did not answer ({status ?? "unknown"}).
    </p>
  );
}

interface Page<T> {
  items: T[];
  next: string | null;
}

// usePages loads a cursor-paged list. Every first load and every load-more carries a generation;
// when key changes (another run, another filter) or the panel unmounts the generation moves on, and
// a response from an older one is dropped. A first-load failure is `error`; a load-more failure is
// `moreError` and leaves what was already loaded in place.
export function usePages<T>(key: string, fetchPage: (cursor: string | null) => Promise<Page<T>>) {
  const [items, setItems] = useState<T[]>([]);
  const [next, setNext] = useState<string | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [moreError, setMoreError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(true);
  const [tries, setTries] = useState(0);
  const gen = useRef(0);
  const busy = useRef(false);
  const fetchRef = useRef(fetchPage);
  fetchRef.current = fetchPage;
  useEffect(() => {
    const mine = ++gen.current;
    busy.current = false;
    setItems([]);
    setNext(null);
    setError(null);
    setMoreError(null);
    setLoading(true);
    fetchRef.current(null).then(
      (p) => {
        if (gen.current !== mine) return;
        setItems(p.items);
        setNext(p.next);
        setLoading(false);
      },
      (e: ApiError) => {
        if (gen.current !== mine) return;
        setError(e);
        setLoading(false);
      },
    );
    return () => {
      gen.current++;
    };
  }, [key, tries]);
  const more = () => {
    if (!next || busy.current) return;
    const mine = gen.current;
    busy.current = true;
    setMoreError(null);
    fetchRef.current(next).then(
      (p) => {
        if (gen.current !== mine) return;
        busy.current = false;
        setItems((l) => [...l, ...p.items]);
        setNext(p.next);
      },
      (e: ApiError) => {
        if (gen.current !== mine) return;
        busy.current = false;
        setMoreError(e);
      },
    );
  };
  return { items, next, error, moreError, loading, more, retry: () => setTries((n) => n + 1) };
}

// TracePanel is the span waterfall, 500 spans a page. Ticked spans become the verdict's evidence;
// a span's name opens its details, named in the URL.
export function TracePanel({ base, evidence, setEvidence, page = "", attempts = [], selected = null }: {
  base: string;
  evidence: EvidenceRef[];
  setEvidence: (refs: EvidenceRef[]) => void;
  page?: string;
  attempts?: Doc[];
  selected?: string | null;
}) {
  const { items: spans, next, error, moreError, loading, more, retry } = usePages<Span>(base, (cursor) =>
    getJSON<{ spans: Span[]; next_cursor?: string }>(`${base}/spans?limit=500${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`).then((p) => ({
      items: list(p.spans),
      next: p.next_cursor ?? null,
    })),
  );
  if (error)
    return (
      <>
        <p className="refusal">{error.message}</p>
        <button onClick={retry}>Try again</button>
      </>
    );
  if (loading) return <p className="loading">Loading spans…</p>;
  if (spans.length === 0) return <p className="muted">No spans.</p>;
  const full = evidence.length >= MAX_EVIDENCE;
  const toggle = (id: string) => setEvidence(evidence.includes(id) ? evidence.filter((r) => r !== id) : [...evidence, id]);
  const sel = selected ? spans.find((s) => s.span_id === selected) : undefined;
  return (
    <>
      <p className="muted">
        Tick spans to cite them in a verdict ({evidence.length} of at most {MAX_EVIDENCE}). A span's name opens its details.
      </p>
      <ul className="waterfall">
        {waterfall(spans).map(({ span, depth, left, width }) => {
          const ticked = evidence.includes(span.span_id);
          return (
            <li key={span.span_id}>
              <label className="cite">
                <input
                  type="checkbox"
                  aria-label={`Cite ${span.name} as evidence`}
                  checked={ticked}
                  disabled={!ticked && full}
                  onChange={() => toggle(span.span_id)}
                />
              </label>
              <button
                type="button"
                className="span-name"
                style={{ paddingLeft: depth * 12 }}
                title={`${span.service} · ${span.span_id}`}
                aria-pressed={span.span_id === selected}
                onClick={() => navigate(runURL(page, { span: span.span_id }))}
              >
                {span.name}
              </button>
              <span className="track">
                <span className="bar" style={{ left: `${left}%`, width: `${width}%` }} />
              </span>
              <span className="dur">{Math.round(span.duration_ms)} ms</span>
            </li>
          );
        })}
      </ul>
      {moreError && <p className="refusal">{moreError.message}</p>}
      {next && <button onClick={more}>{moreError ? "Try loading more spans again" : "Load more spans"}</button>}
      {sel && (
        <SpanPane key={sel.span_id} span={sel} api={base} page={page} attempts={attempts} cited={evidence.includes(sel.span_id)} canCite={!full} onCite={() => toggle(sel.span_id)} />
      )}
    </>
  );
}

const LEVELS = ["", "debug", "info", "warn", "error"];

export function LogsPanel({ base }: { base: string }) {
  const [level, setLevel] = useState("");
  const { items: lines, next, error, moreError, loading, more, retry } = usePages<LogLine>(`${base}|${level}`, (cursor) =>
    getJSON<{ logs: LogLine[]; next_cursor: string | null }>(
      `${base}/logs?limit=100${level ? `&level=${level}` : ""}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`,
    ).then((p) => ({ items: list(p.logs), next: p.next_cursor ?? null })),
  );
  return (
    <>
      <label>
        Level{" "}
        <select value={level} onChange={(e) => setLevel(e.target.value)}>
          {LEVELS.map((l) => (
            <option key={l} value={l}>
              {l || "all"}
            </option>
          ))}
        </select>
      </label>
      {error && (
        <>
          <p className="refusal">{error.message}</p>
          <button onClick={retry}>Try again</button>
        </>
      )}
      <div className="scroll">
        <table>
          <tbody>
            {lines.map((l, i) => (
              <tr key={i}>
                <td className="mono">{l.at}</td>
                <td>{l.level}</td>
                <td>{l.service}</td>
                <td>{l.message}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {loading && <p className="loading">Loading logs…</p>}
      {!error && !loading && lines.length === 0 && <p className="muted">No log lines.</p>}
      {moreError && <p className="refusal">{moreError.message}</p>}
      {next && <button onClick={more}>{moreError ? "Try loading more again" : "Load more"}</button>}
    </>
  );
}

export function ErrorsPanel({ doc }: { doc: Doc }) {
  const errors = list<Doc>(doc.errors);
  if (errors.length === 0) return <p className="muted">No errors.</p>;
  return (
    <div className="scroll">
      <table>
        <tbody>
          {errors.map((e, i) => (
            <tr key={i}>
              <td className="mono">{display(e.at)}</td>
              <td>{display(e.service)}</td>
              <td>{display(e.message)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function AttemptsPanel({ doc }: { doc: Doc }) {
  const attempts = list<Doc>(doc.attempts);
  if (attempts.length === 0) return <p className="muted">No attempts.</p>;
  return (
    <div className="scroll">
      <table>
        <thead>
          <tr>
            <th>Attempt</th>
            <th>State</th>
            <th>Configuration fingerprint</th>
            <th>Harness</th>
            <th>Model</th>
          </tr>
        </thead>
        <tbody>
          {attempts.map((a, i) => (
            <tr key={a.id ?? i}>
              <td className="mono">{display(a.id)}</td>
              <td>{display(a.state)}</td>
              <td className="mono">{display(a.fingerprint?.digest)}</td>
              <td>
                {display(a.fingerprint?.harness)} {display(a.fingerprint?.harness_version)}
              </td>
              <td>{display(a.fingerprint?.model)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// VerdictList shows supersede chains as history: a superseded verdict is struck through and
// links to its successor. Revise is offered on the viewer's own latest rubric verdicts.
export interface Failure {
  subject: string;
  message: string;
}

export function VerdictList({ verdicts, gates, me, onRevise, failures = [], loading = false, onRetry, attempts = [], page = "" }: {
  verdicts: HistoryVerdict[];
  gates: Doc[];
  me: Me;
  onRevise: (v: HistoryVerdict) => void;
  failures?: Failure[];
  loading?: boolean;
  onRetry?: () => void;
  attempts?: Doc[];
  page?: string;
}) {
  const failed =
    failures.length > 0 ? (
      <div className="unavailable" role="alert">
        {failures.map((f) => (
          <p key={f.subject}>
            unavailable: the verdict history for {f.subject} could not be read ({f.message}).
          </p>
        ))}
        {onRetry && <button onClick={onRetry}>Try again</button>}
      </div>
    ) : null;
  if (loading && verdicts.length === 0 && gates.length === 0 && !failed) return <p className="loading">Loading verdicts…</p>;
  if (verdicts.length === 0 && gates.length === 0) return failed ?? <p className="muted">No verdicts yet.</p>;
  return (
    <>
    {failed}
    <ul className="verdicts">
      {gates.map((g) => (
        <li key={g.id} className="verdict">
          <div className="row">
            <strong>Gate</strong> <span className="mono">{display(g.standard)}</span> <span>{display(g.status)}</span>
            <span className="value">{verdictSummary(g.value)}</span> <span className="muted">{display(g.judge)}</span>
          </div>
        </li>
      ))}
      {verdicts.map((v) => (
        <li key={v.id} id={v.id} className={v.superseded_by ? "verdict superseded" : "verdict"}>
          <div className="row">
            <span className="value">
              <strong>{verdictSummary(v.value)}</strong>
            </span>
            <span className="mono">{v.standard}</span>
            <span className="muted">
              {v.judge} ({v.judge_kind})
            </span>
            <span className="muted">{v.created_at}</span>
            {v.subject_kind === "attempt" && <span className="mono">{v.subject_ref}</span>}
            {v.superseded_by && (
              <a href={`#${v.superseded_by}`}>superseded by {v.superseded_by}</a>
            )}
            {list<unknown>(v.evidence_refs).map((r, i) => {
              const ref = r as { session_id?: unknown; seq_from?: unknown; seq_to?: unknown };
              if (typeof r !== "object" || r === null || typeof ref.seq_from !== "number" || typeof ref.seq_to !== "number") return null;
              const from = ref.seq_from;
              const inSession = attempts.filter((x) => x.session_id === ref.session_id);
              const a =
                inSession.find((x) => x.seq_from <= from && (x.seq_to === null || from <= x.seq_to)) ?? inSession[0];
              if (!a) return null;
              return (
                <Link key={i} to={runURL(page, { tab: "transcript", attempt: a.id, seq: seqParam(ref.seq_from, ref.seq_to) })}>
                  transcript {ref.seq_from}–{ref.seq_to} ↗
                </Link>
              );
            })}
            {!v.superseded_by && v.judge === me.principal && v.standard.startsWith("rubric:") && (
              <button onClick={() => onRevise(v)}>Revise</button>
            )}
          </div>
          {v.comment && <p>{v.comment}</p>}
        </li>
      ))}
    </ul>
    </>
  );
}
