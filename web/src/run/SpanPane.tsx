import { useState } from "react";
import { type ApiError, getJSON } from "../api";
import { list } from "../format";
import { useJSON } from "../hooks";
import { Link, navigate } from "../router";
import type { Doc, LogLine, Span } from "../types";
import {
  hasTruncated, itemFirstSeq, itemURL, logsOf, type Part, requestResponse, runURL, type SeqRange, seqParam, showValue, spanRange,
  spanStatus, type TranscriptItem, type TranscriptPage, transcriptURL,
} from "./transcript";

// TranscriptRefusal says why session content is not shown. The rest of the page keeps working.
export function TranscriptRefusal({ error, onRetry }: { error: ApiError; onRetry: () => void }) {
  if (error.status === 403 && error.code === "transcripts_forbidden") return <p className="muted">Transcripts aren't available to this account</p>;
  if (error.status === 503 || error.status === 0)
    return (
      <div className="unavailable" role="alert">
        <p>Transcript unavailable, try again</p>
        <button onClick={onRetry}>Try again</button>
      </div>
    );
  return <p className="refusal">{error.message}</p>;
}

// Value is one field as text; a field the hub cut offers Show full.
export function Value({ v, onFull }: { v: unknown; onFull?: () => void }) {
  if (v === undefined || v === null) return <p className="muted">none</p>;
  return (
    <>
      <pre className="value">{showValue(v)}</pre>
      {onFull && hasTruncated(v) && <button onClick={onFull}>Show full</button>}
    </>
  );
}

// useFullItems holds the whole items read from the item route for one attempt, by first seq. The
// range the items were shown in rides along, so the hub resolves the item the page lists.
export function useFullItems(api: string, attempt: string, range?: { from: number; to: number | null }) {
  const [full, setFull] = useState<Record<number, TranscriptItem>>({});
  const [error, setError] = useState<string | null>(null);
  const load = (it: TranscriptItem) => {
    const from = itemFirstSeq(it);
    setError(null);
    getJSON<{ item: TranscriptItem }>(itemURL(api, attempt, from, range)).then(
      (r) => setFull((f) => ({ ...f, [from]: r.item })),
      (e: ApiError) => setError(e.message),
    );
  };
  const resolve = (it: TranscriptItem) => full[itemFirstSeq(it)] ?? it;
  return { load, resolve, error };
}

// SpanPane is one span's details, opened from the waterfall. It reads session content only now.
export function SpanPane({ span, api, page, attempts, cited, canCite, onCite }: {
  span: Span;
  api: string;
  page: string;
  attempts: Doc[];
  cited: boolean;
  canCite: boolean;
  onCite: () => void;
}) {
  const range = spanRange(span, attempts);
  const attempt = range ? attempts.find((a) => a.id === range.attempt) : undefined;
  const logs = useJSON<{ logs: LogLine[] }>(`${api}/logs?limit=500`);
  const lines = logsOf(list(logs.data?.logs), span);
  return (
    <aside className="pane" aria-label="Span details">
      <div className="pane-head">
        <h2>{span.name}</h2>
        <button onClick={() => navigate(runURL(page, {}))}>Close</button>
      </div>
      <dl className="facts">
        <div>
          <dt>Service</dt>
          <dd>{span.service}</dd>
        </div>
        <div>
          <dt>Start</dt>
          <dd>{span.start}</dd>
        </div>
        <div>
          <dt>Duration</dt>
          <dd>{Math.round(span.duration_ms)} ms</dd>
        </div>
        <div>
          <dt>Status</dt>
          <dd>{spanStatus(span)}</dd>
        </div>
      </dl>
      <h3>Attributes</h3>
      <div className="scroll">
        <table>
          <tbody>
            {Object.keys(span.attributes)
              .sort()
              .map((k) => (
                <tr key={k}>
                  <td className="mono">{k}</td>
                  <td className="mono">{span.attributes[k]}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
      <h3>Logs</h3>
      {logs.error ? (
        <p className="refusal">{logs.error.message}</p>
      ) : lines.length > 0 ? (
        <div className="scroll">
          <table>
            <tbody>
              {lines.map((l, i) => (
                <tr key={i}>
                  <td className="mono">{l.at}</td>
                  <td>{l.level}</td>
                  <td>{l.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="muted">{logs.loading ? "Loading logs…" : "No log lines in this span."}</p>
      )}
      {range ? (
        <SpanContent api={api} page={page} range={range} spanName={span.name} running={attempt?.seq_to === null} />
      ) : (
        <p className="muted">no session content for this span</p>
      )}
      <div className="actions">
        <button aria-pressed={cited} disabled={!cited && !canCite} onClick={onCite}>
          {cited ? "Cited" : "Cite"}
        </button>
      </div>
    </aside>
  );
}

function SpanContent({ api, page, range, spanName, running }: { api: string; page: string; range: SeqRange; spanName: string; running: boolean }) {
  const tx = useJSON<TranscriptPage>(transcriptURL(api, range.attempt, { from: range.from, to: range.to }));
  const full = useFullItems(api, range.attempt, { from: range.from, to: range.to });
  let body;
  if (tx.error) body = <TranscriptRefusal error={tx.error} onRetry={tx.reload} />;
  else if (!tx.data) body = <p className="loading">Loading session content…</p>;
  else {
    const items = list(tx.data.items).map(full.resolve);
    const { request, response } = requestResponse(spanName, items, range.from);
    const parts = (ps: Part[]) =>
      ps.length === 0 ? <p className="muted">none in this span</p> : ps.map((p, i) => <Value key={i} v={p.value} onFull={() => full.load(p.item)} />);
    const n = tx.data.redactions ?? 0;
    body = (
      <>
        <h3>Request</h3>
        {parts(request)}
        <h3>Response</h3>
        {parts(response)}
        <p className="muted">
          {n} value{n === 1 ? "" : "s"} redacted
        </p>
        {full.error && <p className="refusal">{full.error}</p>}
      </>
    );
  }
  return (
    <section aria-label="Session content">
      {running && <p className="muted">Attempt still running; refresh for more</p>}
      {body}
      {tx.data && tx.data.next_cursor !== null && <p className="muted">More steps than shown here; open the transcript for the rest.</p>}
      <p>
        <Link to={runURL(page, { tab: "transcript", attempt: range.attempt, seq: seqParam(range.from, range.to) })}>Open in transcript ↗</Link>
      </p>
    </section>
  );
}
