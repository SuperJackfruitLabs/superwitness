import { ErrorView, NotFound } from "./Edge";
import { useJSON } from "./hooks";
import { match, useLocation } from "./router";
import { RubricPage } from "./rubrics/RubricPage";
import { RubricsPage } from "./rubrics/RubricsPage";
import { RunViewPage } from "./run/RunViewPage";
import { RunsPage } from "./runs/RunsPage";
import { Shell } from "./Shell";
import type { Me } from "./types";

export function App() {
  const me = useJSON<Me>("/v1/me");
  const { path, query } = useLocation();
  if (me.error) return <ErrorView error={me.error} next={query ? `${path}?${query}` : path} />;
  if (!me.data) return <p className="loading">Loading…</p>;
  const route = match(path);
  let page;
  switch (route.name) {
    case "runs":
      page = <RunsPage query={query} />;
      break;
    case "run":
      page = <RunViewPage board={route.board} run={route.run} query={query} me={me.data} />;
      break;
    case "rubrics":
      page = <RubricsPage />;
      break;
    case "rubric":
      page = <RubricPage id={route.id} version={route.version} />;
      break;
    default:
      page = <NotFound />;
  }
  return <Shell me={me.data}>{page}</Shell>;
}
