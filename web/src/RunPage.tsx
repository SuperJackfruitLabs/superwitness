import { useEffect, useState } from "react";
import { display, emptyNote, isAuthStatus, list, statusTone, tokenKey } from "./format";

// The RunDocument JSON (internal/join/document.go). Read loosely: the page shows what it gets.
type Doc = Record<string, any>;

function readToken(): string {
  try {
    return sessionStorage.getItem(tokenKey) ?? "";
  } catch {
    return "";
  }
}

class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
  }
}

export function RunView({
  doc, logs, logsError, onLoadLogs,
}: {
  doc: Doc;
  logs: Doc[] | null;
  logsError: string | null;
  onLoadLogs: () => void;
}) {
  const sources = (doc.sources ?? {}) as Record<string, string>;
  const run = doc.run ?? {};
  const attempts = list<Doc>(doc.attempts);
  const traceIds = list<string>(doc.trace?.trace_ids);
  const errors = list<Doc>(doc.errors);
  const verdicts = list<Doc>(doc.verdicts);
  const cost = doc.cost ?? {};
  const shown = logs?.length ?? 0;
  return (
    <main>
      <h1><code>{display(run.ref)}</code></h1>
      <p>
        {display(run.card?.title)} · stage <b>{display(run.stage)}</b> · state <b>{display(run.state)}</b> · agent{" "}
        <code>{display(run.agent)}</code> · {display(run.started_at)} → {display(run.ended_at)}
      </p>

      <h2>Sources</h2>
      <p>
        {Object.entries(sources).map(([k, v]) => (
          <span key={k} className={statusTone(v)} style={{ marginRight: 12 }}>{k}: {v}</span>
        ))}
      </p>

      <h2>Attempts</h2>
      {attempts.length === 0 ? <p className="muted">{emptyNote(sources, ["superpipeline", "agentpod"])}</p> : (
        <table>
          <thead><tr><th>attempt</th><th>station</th><th>state</th><th>fingerprint</th><th>harness</th><th>model</th><th>spans</th></tr></thead>
          <tbody>
            {attempts.map((a, i) => (
              <tr key={a.id ?? i}>
                <td className="mono">{display(a.id)}</td><td className="mono">{display(a.station)}</td><td>{display(a.state)}</td>
                <td className="mono">{display(a.fingerprint?.digest)}</td>
                <td>{display(a.fingerprint?.harness)} {display(a.fingerprint?.harness_version)}</td>
                <td>{display(a.fingerprint?.model)}</td>
                <td>{display(a.span_count)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>Trace</h2>
      <p>status <b>{display(doc.trace?.status)}</b> · sampled {display(doc.trace?.sampled)}</p>
      {traceIds.length === 0 ? <p className="muted">{emptyNote(sources, ["traces"])}</p> : (
        <ul>{traceIds.map((id) => <li key={id}><code>{id}</code></li>)}</ul>
      )}

      <h2>Errors</h2>
      {errors.length === 0 ? <p className="muted">{emptyNote(sources, ["errors"])}</p> : (
        <table>
          <tbody>
            {errors.map((e, i) => (
              <tr key={i}><td>{display(e.at)}</td><td>{display(e.service)}</td><td>{display(e.message)}</td><td className="mono">{display(e.trace_id)}</td></tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>Verdicts</h2>
      {verdicts.length === 0 ? <p className="muted">{emptyNote(sources, ["verdicts"])}</p> : (
        <table>
          <thead><tr><th>kind</th><th>source</th><th>status</th><th>value</th><th>judge</th><th>judge kind</th><th>standard</th><th>at</th></tr></thead>
          <tbody>
            {verdicts.map((v, i) => (
              <tr key={v.id ?? i}>
                <td>{display(v.kind)}</td><td>{display(v.source)}</td><td>{display(v.status)}</td><td className="mono">{display(v.value)}</td>
                <td className="mono">{display(v.judge)}</td><td>{display(v.judge_kind)}</td>
                <td className="mono">{display(v.standard)}</td><td>{display(v.at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>Cost</h2>
      <p>{display(cost.status)} · in {display(cost.input_tokens)} · out {display(cost.output_tokens)} · usd {display(cost.usd)}</p>

      <h2>Logs ({display(doc.log_count)})</h2>
      {logsError && <p className="bad">Could not load logs: {logsError}</p>}
      {logs === null ? (
        <button onClick={onLoadLogs}>Load first 100</button>
      ) : (
        <>
          <p className="muted">showing {shown} of {display(doc.log_count)}</p>
          {logs.length === 0 ? <p className="muted">{emptyNote(sources, ["logs"])}</p> : (
            <table>
              <tbody>
                {logs.map((l, i) => (
                  <tr key={i}><td>{display(l.at)}</td><td className={l.level === "error" ? "bad" : ""}>{display(l.level)}</td><td>{display(l.service)}</td><td>{display(l.message)}</td></tr>
                ))}
              </tbody>
            </table>
          )}
          {logsMore(logs) && <button onClick={onLoadLogs}>Load more</button>}
        </>
      )}
    </main>
  );
}

// The next_cursor of the most recent page is stored on the array so RunView stays a pure view.
function logsMore(logs: Doc[]): boolean {
  return Boolean((logs as Doc[] & { next?: string | null }).next);
}

export function RunPage({ board, run }: { board: string; run: string }) {
  const [token, setToken] = useState(readToken);
  const [draft, setDraft] = useState("");
  const [doc, setDoc] = useState<Doc | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [logs, setLogs] = useState<Doc[] | null>(null);
  const [logsError, setLogsError] = useState<string | null>(null);
  const base = `/v1/runs/superpipeline/${board}/${run}`;

  function forgetToken() {
    try { sessionStorage.removeItem(tokenKey); } catch { /* private mode */ }
    setToken("");
    setDoc(null);
    setLogs(null);
  }

  async function get(path: string): Promise<Doc> {
    const r = await fetch(path, { headers: { Authorization: `Bearer ${token}` } });
    const body = await r.json().catch(() => ({}));
    if (!r.ok) throw new ApiError(body?.error?.code ?? `HTTP ${r.status}`, r.status);
    return body;
  }

  useEffect(() => {
    if (!token) return;
    setError(null);
    get(base).then(setDoc, (e) => {
      if (e instanceof ApiError && isAuthStatus(e.status)) {
        forgetToken();
        setError(`token rejected (${e.message}); paste a valid hub token`);
      } else {
        setError(String(e.message ?? e));
      }
    });
  }, [token, base]);

  function loadLogs() {
    const prev = logs as (Doc[] & { next?: string | null }) | null;
    const cursor = prev?.next ? `&cursor=${encodeURIComponent(prev.next)}` : "";
    setLogsError(null);
    get(`${base}/logs?limit=100${cursor}`).then(
      (p) => {
        const merged: Doc[] & { next?: string | null } = [...(prev ?? []), ...list<Doc>(p.logs)];
        merged.next = p.next_cursor ?? null;
        setLogs(merged);
      },
      (e) => {
        if (e instanceof ApiError && isAuthStatus(e.status)) {
          forgetToken();
          setError(`token rejected (${e.message}); paste a valid hub token`);
        } else {
          setLogsError(String(e.message ?? e));
        }
      },
    );
  }

  if (!token) {
    return (
      <main>
        <h1>{board} / {run}</h1>
        {error && <p className="bad">{error}</p>}
        <p>Paste a hub token whose audience is this superwitness. It is kept for this tab only.</p>
        <input value={draft} onChange={(e) => setDraft(e.target.value)} aria-label="hub token" />{" "}
        <button
          onClick={() => {
            try { sessionStorage.setItem(tokenKey, draft.trim()); } catch { /* private mode */ }
            setToken(draft.trim());
          }}
        >
          Open run
        </button>
      </main>
    );
  }
  if (error) return <main><h1>{board} / {run}</h1><p className="bad">Could not load: {error}</p></main>;
  if (!doc) return <main><p className="muted">Loading…</p></main>;
  return <RunView doc={doc} logs={logs} logsError={logsError} onLoadLogs={loadLogs} />;
}
