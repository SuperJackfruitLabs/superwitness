import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RunPage } from "./RunPage";
import { parseRunPath } from "./format";

function Home() {
  return (
    <main>
      <h1>superwitness</h1>
      <p>
        Open <code>/runs/superpipeline/&lt;board&gt;/&lt;run&gt;</code>. Agents and scripts use <code>/v1</code> or{" "}
        <code>/mcp</code>.
      </p>
    </main>
  );
}

const ref = parseRunPath(window.location.pathname);
createRoot(document.getElementById("root")!).render(
  <StrictMode>{ref ? <RunPage board={ref.board} run={ref.run} /> : <Home />}</StrictMode>,
);
