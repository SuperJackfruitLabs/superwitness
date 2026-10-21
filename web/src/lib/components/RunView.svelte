<script lang="ts">
  import { fetched } from "../data/fetched.svelte";
  import { createEvidence } from "../evidence.svelte";
  import { list } from "../format";
  import { navigate } from "../nav";
  import { TAB_LABEL, TAB_SOURCE, TABS, tabOf } from "../run/facts";
  import { history as readHistory } from "../run/history.svelte";
  import type { Doc, HistoryVerdict, Me } from "../types";
  import Loading from "../ui/Loading.svelte";
  import Tabs from "../ui/Tabs.svelte";
  import EvidencePanel from "./EvidencePanel.svelte";
  import AttemptsPanel from "./AttemptsPanel.svelte";
  import EdgePage from "./EdgePage.svelte";
  import ErrorsPanel from "./ErrorsPanel.svelte";
  import LogsPanel from "./LogsPanel.svelte";
  import RunHeader from "./RunHeader.svelte";
  import SourceUnavailable from "./SourceUnavailable.svelte";
  import TracePanel from "./TracePanel.svelte";
  import TranscriptPanel from "./TranscriptPanel.svelte";
  import VerdictDrawer from "./VerdictDrawer.svelte";
  import VerdictList from "./VerdictList.svelte";

  // RunView is one run's page. The route keys it by board/run, so nothing ticked or typed on one
  // run (evidence, an open drawer) can reach another. The tab, the open span, the attempt and the
  // transcript range live in the URL (query).
  let { board, run, query, me = null }: { board: string; run: string; query: string; me?: Me | null } = $props();

  const base = $derived(`/runs/superpipeline/${board}/${run}`);
  const api = $derived(`/v1${base}`);
  const doc = fetched<Doc>(() => api);
  const params = $derived(new URLSearchParams(query));
  const tab = $derived(tabOf(params.get("tab")));
  const evidence = createEvidence();
  // Every verdict on the run and its attempts, for the Verdicts tab.
  const history = readHistory(() => doc.data ?? null);
  // The verdict drawer: Record verdict opens it empty, Revise with the verdict. A recorded verdict
  // closes it and reads the run and its history again.
  let drawer = $state<{ revising?: HistoryVerdict } | null>(null);

  const tabs = $derived(TABS.map((t) => ({ value: t, label: TAB_LABEL[t], href: t === "trace" ? base : `${base}?tab=${t}` })));
  const source = $derived(TAB_SOURCE[tab]);
  const status = $derived<string | undefined>((doc.data?.sources ?? {})[source]);
</script>

{#if doc.error}
  <EdgePage error={doc.error} next={base + (query ? `?${query}` : "")} retry={doc.reload} />
{:else if !doc.data}
  <Loading />
{:else}
  <article class="sw-run" data-drawer={drawer ? (drawer.revising?.id ?? "") : undefined}>
    <RunHeader {board} {run} doc={doc.data} onrecord={() => (drawer = {})} />
    <Tabs label="Run" items={tabs} value={tab} onactivate={navigate} />
    <div class="sw-run__panel" role="tabpanel" aria-label={TAB_LABEL[tab]}>
      {#if tab === "evidence"}
        <EvidencePanel doc={doc.data} />
      {:else if status !== "ok"}
        <SourceUnavailable {source} {status} />
      {:else if tab === "trace"}
        <TracePanel {api} page={base} attempts={list(doc.data.attempts)} selected={params.get("span")} {evidence} />
      {:else if tab === "transcript"}
        <TranscriptPanel {api} page={base} doc={doc.data} attempt={params.get("attempt")} seq={params.get("seq")} {evidence} />
      {:else if tab === "logs"}
        <LogsPanel {api} />
      {:else if tab === "errors"}
        <ErrorsPanel doc={doc.data} />
      {:else if tab === "verdicts"}
        <VerdictList gates={list<Doc>(doc.data.verdicts).filter((v) => v.kind === "gate")} {history} {me} onrevise={(v) => (drawer = { revising: v })} attempts={list(doc.data.attempts)} page={base} />
      {:else}
        <AttemptsPanel doc={doc.data} />
      {/if}
    </div>
    {#if drawer}
      <VerdictDrawer
        doc={doc.data}
        evidence={evidence.refs}
        revising={drawer.revising}
        onclose={() => (drawer = null)}
        onrecorded={() => {
          drawer = null;
          doc.reload();
          history.retry();
        }}
      />
    {/if}
  </article>
{/if}

<style>
  .sw-run { min-width: 0; }
  /* Daylight: the tab's body sits on a raised card. */
  .sw-run__panel { min-width: 0; margin-top: 16px; padding: 20px 24px 24px; border-radius: var(--vk-radius-lg); background: var(--vk-color-surface); box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); }
  /* The transcript's steps are cards of their own. */
  .sw-run__panel:has(:global(.sw-tx)) { padding: 0; background: none; box-shadow: none; }
  :global(:where([data-vibe="paper"])) .sw-run__panel,
  :global(:where([data-vibe="quiet"])) .sw-run__panel { margin-top: 22px; padding: 0; border-radius: 0; background: none; box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-run__panel { margin-top: 28px; padding: 0; border-radius: 0; background: none; box-shadow: none; }
  :global(:where([data-vibe="quiet"])) .sw-run__panel { margin-top: var(--vk-space-4, 16px); }
  /* An open span pane: Daylight and Quiet make room for it, so the whole shell (sections, account,
     page) narrows beside it; Paper and Studio float it over the page. */
  @media (min-width: 1100px) {
    :global([data-vibe="daylight"] .sw-shell:has(.sw-inspector)) { margin-right: 496px; }
    :global([data-vibe="quiet"] .sw-shell:has(.sw-inspector)) { margin-right: 480px; }
  }
  @media (max-width: 759px) {
    :global(:where([data-vibe="daylight"])) .sw-run__panel { padding: 14px 12px 16px; }
  }
</style>
