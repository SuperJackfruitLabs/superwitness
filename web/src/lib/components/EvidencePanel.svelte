<script lang="ts">
  import type { Doc } from "../types";
  import { list } from "../format";
  import EvidenceFields from "./EvidenceFields.svelte";
  let { doc }: { doc: Doc } = $props();
  const sections = [ ["fingerprints", "Historical fingerprints"], ["attempts", "Attempt history"], ["children", "Child work"], ["tools", "Tool receipts"], ["checks", "Checks"], ["integrations", "Integrations"], ["artifacts", "Artifact dispositions"], ["usage", "Usage and cost coverage"], ["contributors", "Contributors"], ["coverage", "Evidence coverage"] ];
</script>

<section aria-labelledby="evidence-title">
  <h2 id="evidence-title">Execution evidence</h2>
  <p>Source snapshots describe recorded work. Historical fingerprints do not establish current readiness. Proposed artifacts and child work do not establish integration; inspect integration and check states.</p>
  <p>Execution source: <strong>{doc.execution?.status ?? "unavailable"}</strong></p>
  {#if doc.execution?.status === "ok" && doc.execution.evidence}
    <EvidenceFields value={{ schema_version: doc.execution.evidence.schema_version, as_of: doc.execution.evidence.as_of }} />
    {#each list<Doc>(doc.execution.evidence.executions) as execution}
      <section class="execution" aria-label={`Execution ${execution.task?.session_id ?? "unknown"}`}>
        <h3>Session {execution.task?.session_id ?? "unknown"}</h3>
        <EvidenceFields value={{ as_of: execution.as_of, binding: execution.binding, task: execution.task }} />
        {#each sections as [key, title]}
          <details><summary>{title}</summary>
            {#if key === "usage"}<p>Ledger values are not a provider bill. Estimated values and partial subtotals retain their coverage below; unknown observations are not zero.</p>{/if}
            <EvidenceFields value={execution[key]} />
          </details>
        {/each}
      </section>
    {:else}<p>No executions recorded in this snapshot.</p>{/each}
  {:else}<p>Execution evidence unavailable. Legacy runs may have no execution snapshot.</p>{/if}

  <h2>Immutable Library exports</h2>
  <p>AgentPod verification attests downloaded source bytes and their identity; it does not establish fresh work or successful product effects. Metadata manifests contain metadata only. Artifact bundles identify selected files; paths and digests below are proof metadata, not fetched file contents.</p>
  {#each list<Doc>(doc.execution_exports) as item}
    <section class="export" aria-label={`Export ${item.export_id}`}>
      <h3>{item.export_id}</h3>
      <p>AgentPod proof: <strong>{item.status ?? "unavailable"}</strong></p>
      <EvidenceFields value={{ reference_id: item.reference_id }} />
      {#if item.status === "ok" && item.receipt}
        <p>{item.receipt.artifact_kind === "execution-artifact-bundle" ? "Selected artifact files bundle" : "Metadata-only manifest"}</p>
        <EvidenceFields value={item.receipt} />
      {:else}<p>Verified receipt unavailable. Pipeline hints are not verification.</p>{/if}
    </section>
  {:else}<p>No export proofs recorded.</p>{/each}

  <h2>Pipeline work provenance</h2>
  <p>Pipeline source: <strong>{doc.work_evidence?.status ?? "unavailable"}</strong>. Reference URLs and executor paths are unverified hints; this panel does not follow them.</p>
  {#if doc.work_evidence?.status === "ok" && doc.work_evidence.evidence}
    <details><summary>Work history, completion and references</summary><EvidenceFields value={doc.work_evidence.evidence} /></details>
  {:else}<p>Work provenance unavailable.</p>{/if}
  <h2>Independent source coverage</h2>
  <EvidenceFields value={doc.sources ?? { status: "unknown" }} />
</section>

<style>
  h2 { font-size: 1.1rem; margin: 24px 0 8px; } h2:first-child { margin-top: 0; } h3 { font-size: 1rem; overflow-wrap: anywhere; }
  p { line-height: 1.6; } .execution, .export { border-top: 1px solid var(--vk-color-line); margin-top: 16px; padding-top: 8px; }
  summary { cursor: pointer; padding: 12px 0; font-weight: 600; } details { border-bottom: 1px solid var(--vk-color-line); }
</style>
