import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/svelte";
import EvidencePanel from "./EvidencePanel.svelte";
afterEach(cleanup);

describe("execution evidence reading", () => {
  it("keeps legacy and independently unavailable sections explicit", () => {
    render(EvidencePanel, { doc: { execution: { status: "timeout", evidence: { executions: [{ task: { session_id: "hidden-session" } }] } }, sources: { traces: "ok", logs: "timeout" } } });
    expect(screen.getByText(/Legacy runs/)).toBeTruthy();
    expect(screen.queryByText("hidden-session")).toBeNull();
    expect(screen.getByText("No export proofs recorded.")).toBeTruthy();
    expect(screen.getByText("Work provenance unavailable.")).toBeTruthy();
    expect(screen.getByText("traces")).toBeTruthy();
  });

  it("exposes bound identities, check states, coverage and immutable selected-file proofs without following URLs", () => {
    render(EvidencePanel, { doc: {
      execution: { status: "ok", evidence: { schema_version: "agentpod-execution-evidence/v1", as_of: "snapshot-time", executions: [{
        task: { session_id: "session-one", task_id: "task-one", plan_digest: "plan-digest" }, binding: { principal_id: "prn_agent01", run: { epoch: 3 } },
        checks: [{ id: "check-one", state: "failed", workspace_digest: "workspace-digest" }], artifacts: [{ id: "artifact-one", disposition: "proposed" }],
        usage: { status: "partial", recorded_subtotal: { cost_usd: 0.2 }, coverage: { unknown: 2 } }, coverage: { truncated: ["tools"], unavailable: ["fingerprints"] },
      }] } },
      execution_exports: [{ export_id: "export-one", status: "ok", receipt: { artifact_kind: "execution-artifact-bundle", library: { item_id: "library-one", version: 4 }, artifact_files: [{ path: "artifacts/file.json", sha256: "file-digest", disposition: "proposed" }] } }],
      work_evidence: { status: "ok", evidence: { provenance: { references: [{ url: "https://untrusted.example/" }] } } },
    } });
    const text = document.body.textContent ?? "";
    for (const value of ["session-one", "prn_agent01", "plan-digest", "workspace-digest", "failed", "proposed", "partial", "library-one", "file-digest", "artifacts/file.json"]) expect(text).toContain(value);
    expect(text).toContain("Historical fingerprints do not establish current readiness");
    expect(text).toContain("not a provider bill");
    expect(document.querySelectorAll("a")).toHaveLength(0);
  });

  it("does not display unverified export receipt payloads", () => {
    render(EvidencePanel, { doc: { execution_exports: [{ export_id: "export-one", status: "unavailable", receipt: { library: { item_id: "unverified-item" } } }] } });
    expect(document.body.textContent).not.toContain("unverified-item");
    expect(screen.getByText(/Verified receipt unavailable/)).toBeTruthy();
  });
});
