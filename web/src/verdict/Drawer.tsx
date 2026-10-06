import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, postJSON } from "../api";
import { list } from "../format";
import { useJSON } from "../hooks";
import type { Doc, EvidenceRef, HistoryVerdict, Rubric } from "../types";
import { refusalText } from "./messages";
import { initialInput, type Input, supported, valueOf } from "./scale";
import { ScaleInput } from "./ScaleInput";

export const MAX_COMMENT = 10000;
export const MAX_EVIDENCE = 100;

// VerdictDrawer records a verdict on the run, or on one of its attempts, against a rubric. The
// idempotency key is made when the drawer opens, so a double click or a retry records one verdict.
// citedEvidence is what a verdict cites: when revising, the earlier verdict's evidence first, then
// any spans ticked now, without repeats. More than MAX_EVIDENCE blocks the save.
export function citedEvidence(ticked: EvidenceRef[], revising?: HistoryVerdict): unknown[] {
  const out: unknown[] = [];
  const seen = new Set<string>();
  for (const ref of [...(Array.isArray(revising?.evidence_refs) ? revising.evidence_refs : []), ...ticked]) {
    const k = JSON.stringify(ref);
    if (!seen.has(k)) {
      seen.add(k);
      out.push(ref);
    }
  }
  return out;
}

// evidenceSummary is the drawer's evidence line: what is cited, and where it came from.
export function evidenceSummary(evidence: unknown[], kept: number): string {
  if (evidence.length === 0) return "none; tick spans in the Trace tab or cite steps in the Transcript tab";
  const added = evidence.slice(kept);
  const spans = added.filter((r) => typeof r === "string").length;
  const steps = added.length - spans;
  const stepText = `${steps} step${steps === 1 ? "" : "s"} from the transcript`;
  const parts = [...(spans > 0 ? [`${spans} ticked in the Trace tab`] : []), ...(steps > 0 ? [stepText] : [])];
  if (kept > 0) return [`${evidence.length} cited: ${kept} from the verdict being revised`, ...parts].join(", ");
  if (steps === 0) return `${spans} span${spans === 1 ? "" : "s"} ticked in the Trace tab`;
  return `${evidence.length} cited: ${parts.join(", ")}`;
}

export function VerdictDrawer({ doc, evidence: ticked, revising, onClose, onRecorded }: {
  doc: Doc;
  evidence: EvidenceRef[];
  revising?: HistoryVerdict;
  onClose: () => void;
  onRecorded: () => void;
}) {
  const rubrics = useJSON<{ rubrics: Rubric[] }>("/v1/rubrics");
  const evidence = citedEvidence(ticked, revising);
  const kept = citedEvidence([], revising).length; // how many come from the verdict being revised
  const [key] = useState(() => crypto.randomUUID());
  const runRef: string = doc.run?.ref;
  const attempts = list<Doc>(doc.attempts).map((a) => String(a.id));
  const [subject, setSubject] = useState(revising ? `${revising.subject_kind}|${revising.subject_ref}` : `run|${runRef}`);
  const [standard, setStandard] = useState(revising?.standard ?? "");
  const [input, setInput] = useState<Input | null>(null);
  const [comment, setComment] = useState(revising?.comment ?? "");
  const [sending, setSending] = useState(false);
  const [refusal, setRefusal] = useState<string | null>(null);

  const usable = (rubrics.data?.rubrics ?? []).filter((r) => supported(r.recognised_scale));
  const rubric = rubrics.data?.rubrics.find((r) => r.standard === standard);
  const scale = supported(rubric?.recognised_scale) ? rubric.recognised_scale : null;
  useEffect(() => {
    setInput(scale ? initialInput(scale, revising?.standard === standard ? revising.value : undefined) : null);
  }, [standard, scale?.kind]);

  // While a save is in flight the drawer cannot be closed: the verdict may still be recorded, and
  // the person should see the outcome.
  const panel = useRef<HTMLElement>(null);
  const sendingRef = useRef(false);
  sendingRef.current = sending;
  const close = useCallback(() => {
    if (!sendingRef.current) onClose();
  }, [onClose]);

  // Focus moves into the drawer on open, stays inside while it is open, and returns to whatever
  // opened it on close.
  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null;
    panel.current?.focus();
    return () => opener?.focus?.();
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") return close();
      if (e.key !== "Tab" || !panel.current) return;
      const f = [...panel.current.querySelectorAll<HTMLElement>("a[href], button, input, select, textarea, [tabindex]")].filter(
        (x) => !(x as HTMLButtonElement).disabled && x.tabIndex >= 0,
      );
      if (f.length === 0) return e.preventDefault();
      const first = f[0];
      const last = f[f.length - 1];
      const at = document.activeElement;
      if (!panel.current.contains(at) || (e.shiftKey && (at === first || at === panel.current))) {
        e.preventDefault();
        (e.shiftKey ? last : first).focus();
      } else if (!e.shiftKey && at === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [close]);

  const value = scale && input ? valueOf(scale, input) : null;
  const commentLen = [...comment].length; // the server counts characters, not UTF-16 units
  const tooMuchEvidence = evidence.length > MAX_EVIDENCE;
  const ready = value !== null && commentLen <= MAX_COMMENT && !tooMuchEvidence && !sending;

  async function send() {
    if (!ready) return;
    const cut = subject.indexOf("|");
    const subjectKind = subject.slice(0, cut);
    const subjectRef = subject.slice(cut + 1);
    setSending(true);
    setRefusal(null);
    try {
      await postJSON("/v1/verdicts", {
        idempotency_key: key,
        subject_kind: subjectKind,
        subject_ref: subjectRef,
        standard,
        value,
        ...(comment ? { comment } : {}),
        ...(evidence.length ? { evidence_refs: evidence } : {}),
        ...(revising ? { supersedes: revising.id } : {}),
      });
      onRecorded();
    } catch (e) {
      setRefusal(e instanceof ApiError ? refusalText(e) : String(e));
      setSending(false);
    }
  }

  return (
    <>
      <div className="backdrop" onClick={close} />
      <aside className="drawer" ref={panel} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby="drawer-title">
        <h2 id="drawer-title">{revising ? "Revise verdict" : "Record verdict"}</h2>
        <label>
          Subject
          <select value={subject} disabled={!!revising} onChange={(e) => setSubject(e.target.value)}>
            <option value={`run|${runRef}`}>This run</option>
            {attempts.map((a) => (
              <option key={a} value={`attempt|${a}`}>
                Attempt {a}
              </option>
            ))}
          </select>
        </label>
        <label>
          Rubric
          <select value={standard} disabled={!!revising} onChange={(e) => setStandard(e.target.value)}>
            <option value="">Choose a rubric…</option>
            {(rubrics.data?.rubrics ?? []).map((r) => (
              <option key={r.standard} value={r.standard} disabled={!supported(r.recognised_scale)}>
                {r.name} ({r.standard}){supported(r.recognised_scale) ? "" : ": scale not supported in the app"}
              </option>
            ))}
          </select>
        </label>
        {rubrics.data && usable.length === 0 && <p className="muted">No rubric has a scale the app supports. Add one with superwitness rubric-add.</p>}
        {scale && input && <ScaleInput scale={scale} input={input} onChange={setInput} />}
        <label>
          Comment (optional)
          <textarea value={comment} onChange={(e) => setComment(e.target.value)} />
          <span className="muted">
            {commentLen} / {MAX_COMMENT}
          </span>
        </label>
        <p className="muted">
          Evidence:{" "}
          {evidenceSummary(evidence, kept)}
        </p>
        {tooMuchEvidence && (
          <p className="refusal" role="alert">
            A verdict cites at most {MAX_EVIDENCE} spans{kept > 0 ? `, the revised verdict's ${kept} included` : ""}; untick some in the Trace tab
          </p>
        )}
        {refusal && (
          <p className="refusal" role="alert">
            {refusal}
          </p>
        )}
        <div className="actions">
          <button onClick={close} disabled={sending}>Cancel</button>
          <button className="primary" disabled={!ready} onClick={send}>
            {sending ? "Saving…" : "Save verdict"}
          </button>
        </div>
      </aside>
    </>
  );
}
