import { display, runTone, timing, verdictSummary } from "../format";
import { Link } from "../router";
import type { RegistryRun } from "../types";

// runHref is the run's page, for the sources that have one.
export function runHref(r: RegistryRun): string | null {
  if (r.source !== "superpipeline" || !/^[A-Za-z0-9_-]{1,128}\/[A-Za-z0-9_-]{1,128}$/.test(r.external_ref)) return null;
  return `/runs/superpipeline/${r.external_ref}`;
}

export function StatusPill({ status }: { status: string }) {
  return <span className={`pill ${runTone(status)}`}>{status}</span>;
}

export function RunCard({ run, now }: { run: RegistryRun; now: Date }) {
  const href = runHref(run);
  const v = run.latest_verdict;
  const body = (
    <>
      <div className="card-top">
        <StatusPill status={run.status} />
        <span className="card-title">{run.title ?? <code>{run.external_ref}</code>}</span>
      </div>
      <div className="card-meta">
        {run.scope && <span>{display(run.scope.name ?? run.scope.id)}</span>}
        {run.executor && <span>{display(run.executor.name ?? run.executor.id)}</span>}
        <span>{timing(run, now)}</span>
        <span>{v ? `verdict: ${verdictSummary(v.value)}` : "no verdict"}</span>
      </div>
    </>
  );
  return href ? (
    <Link to={href} className="card">
      {body}
    </Link>
  ) : (
    <div className="card" title="This source has no run page yet">
      {body}
    </div>
  );
}
