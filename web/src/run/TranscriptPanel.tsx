import { useEffect, useRef } from "react";
import { getJSON } from "../api";
import { list } from "../format";
import { useJSON } from "../hooks";
import { Link, navigate } from "../router";
import type { Doc, EvidenceRef, Span } from "../types";
import { MAX_EVIDENCE } from "../verdict/Drawer";
import { usePages } from "./Panels";
import { TranscriptRefusal, useFullItems, Value } from "./SpanPane";
import { holds, itemRange, parseSeq, rangeRef, runURL, sameRef, showValue, spanFor, type TranscriptItem, type TranscriptPage, transcriptURL } from "./transcript";

const KIND_LABEL: Record<string, string> = {
  prompt: "Prompt", message: "Agent message", reasoning: "Reasoning", tool_call: "Tool call", permission: "Permission",
  state: "State", error: "Error", other: "Event",
};

// How many pages the tab will load to reach a seq named in the URL.
const MAX_CHASE = 20;

// TranscriptPanel is an attempt read as a conversation. With several attempts, a picker chooses one;
// every read names its attempt.
export function TranscriptPanel({ api, page, doc, attempt, seq, evidence, setEvidence }: {
  api: string;
  page: string;
  doc: Doc;
  attempt: string | null;
  seq: string | null;
  evidence: EvidenceRef[];
  setEvidence: (refs: EvidenceRef[]) => void;
}) {
  const attempts = list<Doc>(doc.attempts);
  if (attempts.length === 0) return <p className="muted">No attempts.</p>;
  const chosen = attempt ? attempts.find((a) => a.id === attempt) : attempts[0];
  return (
    <>
      {attempts.length > 1 && (
        <label className="picker">
          Attempt{" "}
          <select value={chosen?.id ?? ""} onChange={(e) => navigate(runURL(page, { tab: "transcript", attempt: e.target.value }))}>
            {!chosen && <option value="">Choose an attempt…</option>}
            {attempts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.id}
              </option>
            ))}
          </select>
        </label>
      )}
      {chosen ? (
        <AttemptTranscript key={chosen.id} api={api} page={page} attempt={chosen} seq={seq} evidence={evidence} setEvidence={setEvidence} />
      ) : (
        <p className="refusal">This run has no attempt {attempt}.</p>
      )}
    </>
  );
}

function AttemptTranscript({ api, page, attempt, seq, evidence, setEvidence }: {
  api: string;
  page: string;
  attempt: Doc;
  seq: string | null;
  evidence: EvidenceRef[];
  setEvidence: (refs: EvidenceRef[]) => void;
}) {
  const id: string = attempt.id;
  const { items, next, error, moreError, loading, more, retry } = usePages<TranscriptItem>(`${api}|${id}`, (cursor) =>
    getJSON<TranscriptPage>(transcriptURL(api, id, { cursor })).then((p) => ({ items: list(p.items), next: p.next_cursor ?? null })),
  );
  const spans = useJSON<{ spans: Span[] }>(`${api}/spans?limit=500`);
  const full = useFullItems(api, id, { from: attempt.seq_from, to: attempt.seq_to });
  const target = parseSeq(seq);
  const found = target ? items.find((it) => holds(itemRange(it), target.from)) : undefined;
  const chased = useRef(0);
  useEffect(() => {
    if (target && !found && next && !loading && chased.current < MAX_CHASE) {
      chased.current++;
      more();
    }
  }, [seq, found, next, loading, items.length]);
  const targetRef = useRef<HTMLLIElement>(null);
  useEffect(() => {
    targetRef.current?.scrollIntoView?.({ block: "center" });
  }, [found]);

  if (error) return <TranscriptRefusal error={error} onRetry={retry} />;
  if (loading) return <p className="loading">Loading the transcript…</p>;
  const session: string = attempt.session_id;
  return (
    <>
      {attempt.seq_to === null && <p className="muted">Attempt still running; refresh for more</p>}
      {items.length === 0 ? (
        <p className="muted">No session content in this attempt.</p>
      ) : (
        <ol className="transcript">
          {items.map((raw) => {
            const it = full.resolve(raw);
            const r = itemRange(it);
            const s = spanFor(list(spans.data?.spans), [attempt], id, r.from, r.to);
            const ref = rangeRef(session, r.from, r.to);
            const cited = evidence.some((e) => sameRef(e, ref));
            const isTarget = found === raw;
            return (
              <li
                key={`${it.kind}-${r.from}`}
                ref={isTarget ? targetRef : undefined}
                aria-current={isTarget ? "true" : undefined}
                className={it.kind === "tool_call" && it.status === "failed" ? "failed" : undefined}
                data-kind={it.kind}
              >
                <div className="card-head">
                  <strong>{KIND_LABEL[it.kind] ?? it.kind}</strong>
                  <span className="mono muted">{r.from === r.to ? `#${r.from}` : `#${r.from}–${r.to}`}</span>
                  {it.partial && <span className="muted">started earlier</span>}
                  {(it.redactions ?? 0) > 0 && <span className="muted">{it.redactions} redacted</span>}
                  {s && <Link to={runURL(page, { span: s.span_id })}>span ↗</Link>}
                  <button
                    aria-pressed={cited}
                    disabled={!cited && evidence.length >= MAX_EVIDENCE}
                    onClick={() => setEvidence(cited ? evidence.filter((e) => !sameRef(e, ref)) : [...evidence, ref])}
                  >
                    {cited ? "Cited" : "Cite"}
                  </button>
                </div>
                <CardBody item={it} onFull={() => full.load(raw)} />
              </li>
            );
          })}
        </ol>
      )}
      {full.error && <p className="refusal">{full.error}</p>}
      {moreError && <p className="refusal">{moreError.message}</p>}
      {next && <button onClick={more}>{moreError ? "Try loading more again" : "Load more"}</button>}
    </>
  );
}

function CardBody({ item: it, onFull }: { item: TranscriptItem; onFull: () => void }) {
  switch (it.kind) {
    case "prompt":
      return (
        <>
          <Value v={it.text} onFull={onFull} />
          {list(it.images).length > 0 && <p className="muted">Images: {list(it.images).map((i) => i.name).join(", ")}</p>}
        </>
      );
    case "message":
      return <Value v={it.text} onFull={onFull} />;
    case "reasoning":
      return (
        <details>
          <summary>Reasoning</summary>
          <Value v={it.text} onFull={onFull} />
        </details>
      );
    case "tool_call":
      return (
        <details open={it.status === "failed"}>
          <summary>
            {showValue(it.title ?? "tool call")} · {it.tool_kind ?? "other"} · {it.status ?? "unknown"}
          </summary>
          <h4>Input</h4>
          <Value v={it.input} onFull={onFull} />
          <h4>Output</h4>
          <Value v={it.output} onFull={onFull} />
        </details>
      );
    case "permission": {
      const options = Array.isArray(it.options) ? (it.options as { name?: string; optionId?: string }[]) : [];
      return (
        <>
          {it.title !== undefined && <p>{showValue(it.title)}</p>}
          <p>Asked to choose: {options.map((o) => o.name ?? o.optionId).join(" / ") || "no options"}</p>
          <p>
            Outcome: <span className="mono">{it.outcome ?? "pending"}</span>
          </p>
        </>
      );
    }
    case "state":
      return (
        <p>
          {it.status ?? "unknown"}
          {it.reason ? ` · ${it.reason}` : ""}
        </p>
      );
    case "error":
      return (
        <p className="refusal">
          {it.error_kind ?? "error"}: {showValue(it.message ?? "")}
        </p>
      );
    default:
      return <p className="muted">An event of type {it.type ?? it.kind}, shown by type only.</p>;
  }
}
