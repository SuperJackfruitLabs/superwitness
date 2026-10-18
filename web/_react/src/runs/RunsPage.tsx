import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, getJSON } from "../api";
import { ErrorView } from "../Edge";
import { runTone } from "../format";
import { Link } from "../router";
import { type RunPage, STATUSES } from "../types";
import { apiQuery, hasFilters, viewTitle, withStatus } from "./filters";
import { groupByDay } from "./group";
import { RunCard } from "./RunCard";

export function RunsPage({ query }: { query: string }) {
  const search = new URLSearchParams(query);
  const api = apiQuery(search);
  const [pages, setPages] = useState<RunPage[]>([]);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(true);
  const sentinel = useRef<HTMLDivElement>(null);
  // Every request belongs to one filter; a result that arrives after the filter changed is dropped.
  const generation = useRef(0);

  useEffect(() => {
    const mine = ++generation.current;
    const live = () => generation.current === mine;
    setPages([]);
    setError(null);
    setLoading(true);
    getJSON<RunPage>(`/v1/runs${api ? `?${api}` : ""}`).then(
      (p) => live() && (setPages([p]), setLoading(false)),
      (e: ApiError) => live() && (setError(e), setLoading(false)),
    );
  }, [api]);

  const next = pages.length ? pages[pages.length - 1].next_cursor : null;
  const loadMore = useCallback(() => {
    if (!next || loading) return;
    const mine = generation.current;
    setError(null);
    setLoading(true);
    getJSON<RunPage>(`/v1/runs?${api ? `${api}&` : ""}cursor=${encodeURIComponent(next)}`).then(
      (p) => mine === generation.current && (setPages((ps) => [...ps, p]), setLoading(false)),
      (e: ApiError) => mine === generation.current && (setError(e), setLoading(false)),
    );
  }, [api, next, loading]);

  // Scrolling near the end loads the next page. After an error nothing retries by itself: the
  // button does.
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !next || error || typeof IntersectionObserver === "undefined") return;
    const io = new IntersectionObserver((es) => es.some((e) => e.isIntersecting) && loadMore(), { rootMargin: "400px" });
    io.observe(el);
    return () => io.disconnect();
  }, [loadMore, next, error]);

  if (error && pages.length === 0) return <ErrorView error={error} />;
  const runs = pages.flatMap((p) => p.runs);
  const counts = pages[0]?.counts;
  const now = new Date();
  const statuses = search.getAll("status");
  return (
    <section>
      <h1>{viewTitle(search)}</h1>
      {counts && (
        <ul className="chips" aria-label="Runs by status">
          {STATUSES.map((s) => (
            <li key={s}>
              <Link to={withStatus(search, s)} aria-current={statuses.length === 1 && statuses[0] === s ? "true" : undefined}>
                <span className={`pill ${runTone(s)}`}>
                  {s} · {counts[s] ?? 0}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {!loading && runs.length === 0 && (
        <p className="empty">
          {hasFilters(search) ? "No runs in this view." : "No runs reported yet."}{" "}
          <a href="https://docs.superwitness.dev/build/run-registry/">How sources report runs</a>
        </p>
      )}
      {groupByDay(runs, now).map((g) => (
        <section key={g.key} aria-labelledby={`day-${g.key}`}>
          <h2 id={`day-${g.key}`} className="day">
            {g.label}
          </h2>
          <ul className="cards">
            {g.runs.map((r) => (
              <li key={`${r.source}/${r.external_ref}`}>
                <RunCard run={r} now={now} />
              </li>
            ))}
          </ul>
        </section>
      ))}
      <div ref={sentinel} />
      {error && pages.length > 0 && <p className="refusal">{error.message}</p>}
      {next && (
        <button onClick={loadMore} disabled={loading}>
          {loading ? "Loading…" : "Load more"}
        </button>
      )}
    </section>
  );
}
