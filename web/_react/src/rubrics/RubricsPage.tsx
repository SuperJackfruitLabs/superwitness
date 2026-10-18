import { ErrorView } from "../Edge";
import { useJSON } from "../hooks";
import { Link } from "../router";
import type { Rubric } from "../types";

export function RubricsPage() {
  const r = useJSON<{ rubrics: Rubric[] }>("/v1/rubrics");
  if (r.error) return <ErrorView error={r.error} />;
  if (!r.data) return <p className="loading">Loading…</p>;
  return (
    <section>
      <h1>Rubrics</h1>
      {r.data.rubrics.length === 0 ? (
        <p className="empty">No rubrics yet. Record one with superwitness rubric-add.</p>
      ) : (
        <div className="scroll">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Rubric</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {r.data.rubrics.map((x) => (
                <tr key={x.standard}>
                  <td>
                    <Link to={`/rubrics/${x.id}/${x.version}`}>{x.name}</Link>
                    {!x.recognised_scale && <span className="muted"> · scale not supported in the app</span>}
                  </td>
                  <td className="mono">
                    {x.id}@{x.version}
                  </td>
                  <td>{x.created_at.slice(0, 10)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
