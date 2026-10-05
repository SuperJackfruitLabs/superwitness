import { useEffect, useState } from "react";
import { ApiError, getJSON } from "../api";
import { display, list } from "../format";
import type { Doc, HistoryVerdict, LogLine, Me, Span } from "../types";
import { verdictSummary } from "../format";
import { MAX_EVIDENCE } from "../verdict/Drawer";
import { waterfall } from "./facts";

export { MAX_EVIDENCE };

export function Unavailable({ source, status }: { source: string; status: string | undefined }) {
  return (
    <p className="unavailable">
      unavailable: the {source} source did not answer ({status ?? "unknown"}).
    </p>
  );
}

// TracePanel is the span waterfall, 500 spans a page. Ticked spans become the verdict's evidence.
export function TracePanel({ base, evidence, setEvidence }: { base: string; evidence: string[]; setEvidence: (ids: string[]) => void }) {
  const [spans, setSpans] = useState<Span[]>([]);
  const [next, setNext] = useState<string | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const load = (cursor: string | null) =>
    getJSON<{ spans: Span[]; next_cursor?: string }>(`${base}/spans?limit=500${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`).then(
      (p) => {
        setSpans((s) => (cursor ? [...s, ...list(p.spans)] : list(p.spans)));
        setNext(p.next_cursor ?? null);
      },
      setError,
    );
  useEffect(() => {
    load(null);
  }, [base]);
  if (error) return <p className="refusal">{error.message}</p>;
  if (spans.length === 0) return <p className="muted">No spans.</p>;
  const full = evidence.length >= MAX_EVIDENCE;
  return (
    <>
      <p className="muted">
        Tick spans to cite them in a verdict ({evidence.length} of at most {MAX_EVIDENCE}).
      </p>
      <ul className="waterfall">
        {waterfall(spans).map(({ span, depth, left, width }) => {
          const ticked = evidence.includes(span.span_id);
          return (
            <li key={span.span_id}>
              <input
                type="checkbox"
                aria-label={`Cite ${span.name} as evidence`}
                checked={ticked}
                disabled={!ticked && full}
                onChange={() => setEvidence(ticked ? evidence.filter((id) => id !== span.span_id) : [...evidence, span.span_id])}
              />
              <span className="span-name" style={{ paddingLeft: depth * 12 }} title={`${span.service} · ${span.span_id}`}>
                {span.name}
              </span>
              <span className="track">
                <span className="bar" style={{ left: `${left}%`, width: `${width}%` }} />
              </span>
              <span className="dur">{Math.round(span.duration_ms)} ms</span>
            </li>
          );
        })}
      </ul>
      {next && <button onClick={() => load(next)}>Load more spans</button>}
    </>
  );
}

const LEVELS = ["", "debug", "info", "warn", "error"];

export function LogsPanel({ base }: { base: string }) {
  const [level, setLevel] = useState("");
  const [lines, setLines] = useState<LogLine[]>([]);
  const [next, setNext] = useState<string | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const load = (cursor: string | null) =>
    getJSON<{ logs: LogLine[]; next_cursor: string | null }>(
      `${base}/logs?limit=100${level ? `&level=${level}` : ""}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`,
    ).then((p) => {
      setLines((l) => (cursor ? [...l, ...list(p.logs)] : list(p.logs)));
      setNext(p.next_cursor);
    }, setError);
  useEffect(() => {
    setError(null);
    load(null);
  }, [base, level]);
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
      {error && <p className="refusal">{error.message}</p>}
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
      {!error && lines.length === 0 && <p className="muted">No log lines.</p>}
      {next && <button onClick={() => load(next)}>Load more</button>}
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
export function VerdictList({ verdicts, gates, me, onRevise }: { verdicts: HistoryVerdict[]; gates: Doc[]; me: Me; onRevise: (v: HistoryVerdict) => void }) {
  if (verdicts.length === 0 && gates.length === 0) return <p className="muted">No verdicts yet.</p>;
  return (
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
            {!v.superseded_by && v.judge === me.principal && v.standard.startsWith("rubric:") && (
              <button onClick={() => onRevise(v)}>Revise</button>
            )}
          </div>
          {v.comment && <p>{v.comment}</p>}
        </li>
      ))}
    </ul>
  );
}
