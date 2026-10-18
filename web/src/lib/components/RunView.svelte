<script lang="ts">
  import { fetched } from "../data/fetched.svelte";
  import { createEvidence } from "../evidence.svelte";
  import { list } from "../format";
  import { navigate } from "../nav";
  import { TAB_LABEL, TAB_SOURCE, TABS, tabOf } from "../run/facts";
  import type { Doc } from "../types";
  import Loading from "../ui/Loading.svelte";
  import Tabs from "../ui/Tabs.svelte";
  import AttemptsPanel from "./AttemptsPanel.svelte";
  import EdgePage from "./EdgePage.svelte";
  import ErrorsPanel from "./ErrorsPanel.svelte";
  import LogsPanel from "./LogsPanel.svelte";
  import RunHeader from "./RunHeader.svelte";
  import SourceUnavailable from "./SourceUnavailable.svelte";
  import TracePanel from "./TracePanel.svelte";
  import TranscriptPanel from "./TranscriptPanel.svelte";
  import VerdictList from "./VerdictList.svelte";

  // RunView is one run's page. The route keys it by board/run, so nothing ticked or typed on one
  // run (evidence, an open drawer) can reach another. The tab, the open span, the attempt and the
  // transcript range live in the URL (query).
  let { board, run, query }: { board: string; run: string; query: string } = $props();

  const base = $derived(`/runs/superpipeline/${board}/${run}`);
  const api = $derived(`/v1${base}`);
  const doc = fetched<Doc>(() => api);
  const params = $derived(new URLSearchParams(query));
  const tab = $derived(tabOf(params.get("tab")));
  const evidence = createEvidence();
  // The verdict drawer arrives in Part 11; Record verdict opens it.
  let drawer = $state(false);

  const tabs = $derived(TABS.map((t) => ({ value: t, label: TAB_LABEL[t], href: t === "trace" ? base : `${base}?tab=${t}` })));
  const source = $derived(TAB_SOURCE[tab]);
  const status = $derived<string | undefined>((doc.data?.sources ?? {})[source]);
</script>

{#if doc.error}
  <EdgePage error={doc.error} next={base + (query ? `?${query}` : "")} retry={doc.reload} />
{:else if !doc.data}
  <Loading />
{:else}
  <article class="sw-run" data-drawer={drawer ? "" : undefined}>
    <RunHeader {board} {run} doc={doc.data} onrecord={() => (drawer = true)} />
    <Tabs label="Run" items={tabs} value={tab} onactivate={navigate} />
    <div class="sw-run__panel" role="tabpanel" aria-label={TAB_LABEL[tab]}>
      {#if status !== "ok"}
        <SourceUnavailable {source} {status} />
      {:else if tab === "trace"}
        <TracePanel doc={doc.data} {api} page={base} attempts={list(doc.data.attempts)} selected={params.get("span")} {evidence} />
      {:else if tab === "transcript"}
        <TranscriptPanel doc={doc.data} />
      {:else if tab === "logs"}
        <LogsPanel doc={doc.data} />
      {:else if tab === "errors"}
        <ErrorsPanel doc={doc.data} />
      {:else if tab === "verdicts"}
        <VerdictList doc={doc.data} />
      {:else}
        <AttemptsPanel doc={doc.data} />
      {/if}
    </div>
  </article>
{/if}

<style>
  .sw-run { min-width: 0; }
  /* Daylight: the tab's body sits on a raised card. */
  .sw-run__panel { min-width: 0; margin-top: 16px; padding: 20px 24px 24px; border-radius: var(--vk-radius-lg); background: var(--vk-color-surface); box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); }
  :global(:where([data-vibe="paper"])) .sw-run__panel,
  :global(:where([data-vibe="quiet"])) .sw-run__panel { margin-top: 22px; padding: 0; border-radius: 0; background: none; box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-run__panel { margin-top: 28px; padding: 0; border-radius: 0; background: none; box-shadow: none; }
  :global(:where([data-vibe="quiet"])) .sw-run__panel { margin-top: var(--vk-space-4, 16px); }
  @media (max-width: 759px) {
    .sw-run__panel { padding: 14px 12px 16px; }
  }
</style>
