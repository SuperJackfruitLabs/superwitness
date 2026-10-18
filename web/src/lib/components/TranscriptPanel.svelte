<script lang="ts">
  import type { Evidence } from "../evidence.svelte";
  import { list } from "../format";
  import { navigate } from "../nav";
  import { runURL } from "../run/transcript";
  import type { Doc } from "../types";
  import Empty from "../ui/Empty.svelte";
  import ErrorPanel from "../ui/ErrorPanel.svelte";
  import TranscriptAttempt from "./TranscriptAttempt.svelte";

  // TranscriptPanel is an attempt read as a conversation. With several attempts, a picker chooses one
  // (?attempt=); every read names its attempt.
  let { api, page, doc, attempt, seq, evidence }: {
    api: string; page: string; doc: Doc; attempt: string | null; seq: string | null; evidence: Evidence;
  } = $props();

  const attempts = $derived(list<Doc>(doc.attempts));
  const chosen = $derived(attempt ? attempts.find((a) => a.id === attempt) : attempts[0]);
</script>

{#if attempts.length === 0}
  <Empty>No attempts.</Empty>
{:else}
  {#if attempts.length > 1}
    <label class="picker">
      <span>Attempt</span>
      <select value={chosen?.id ?? ""} onchange={(e) => navigate(runURL(page, { tab: "transcript", attempt: e.currentTarget.value }))}>
        {#if !chosen}<option value="">Choose an attempt…</option>{/if}
        {#each attempts as a (a.id)}<option value={a.id}>{a.id}</option>{/each}
      </select>
    </label>
  {/if}
  {#if chosen}
    {#key chosen.id}
      <TranscriptAttempt {api} {page} attempt={chosen} {seq} {evidence} />
    {/key}
  {:else}
    <ErrorPanel message={`This run has no attempt ${attempt}.`} />
  {/if}
{/if}

<style>
  .picker { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; margin: 0 0 14px; font-weight: 700; }
  .picker select {
    min-height: 44px; min-width: 14ch; max-width: 100%; padding: 0 12px; border-radius: var(--vk-radius-sm); border: 2px solid var(--vk-color-line);
    background: var(--vk-color-surface); color: var(--vk-color-text); font: 500 16px var(--vk-font-mono);
  }
  .picker select:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }
  :global(:where([data-vibe="paper"])) .picker span { font: italic 400 18px var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .picker select,
  :global(:where([data-vibe="studio"])) .picker select { border-radius: 0; border-color: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .picker { font-weight: 400; }
</style>
