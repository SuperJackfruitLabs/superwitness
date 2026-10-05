import { ErrorView } from "../Edge";
import { useJSON } from "../hooks";
import { Link } from "../router";
import type { Rubric, Scale } from "../types";

export function describeScale(s: Scale | null): string {
  if (!s) return "Not supported in the app.";
  switch (s.kind) {
    case "decision":
      return `A decision: ${(s.options ?? []).join(", ")}.`;
    case "label":
      return `A label: ${(s.labels ?? []).join(", ")}.`;
    case "score":
      return s.min === 0 && s.max === 1 ? "A score from 0 to 1." : `A score from ${s.min} to ${s.max}, recorded as 0 to 1.`;
    case "text":
      return "Free text.";
  }
}

export function RubricPage({ id, version }: { id: string; version: number }) {
  const r = useJSON<Rubric>(`/v1/rubrics/${encodeURIComponent(id)}/${version}`);
  if (r.error) return <ErrorView error={r.error} />;
  if (!r.data) return <p className="loading">Loading…</p>;
  const x = r.data;
  return (
    <article>
      <p className="mono ref">
        <Link to="/rubrics">Rubrics</Link> · {x.standard}
      </p>
      <h1>{x.name}</h1>
      <p className="muted">
        Created {x.created_at.slice(0, 10)} by <span className="mono">{x.created_by}</span>
      </p>
      <h2>Scale</h2>
      <p>{describeScale(x.recognised_scale)}</p>
      <pre className="mono">{JSON.stringify(x.scale)}</pre>
      <h2>Rubric</h2>
      <pre className="body">{x.body}</pre>
    </article>
  );
}
