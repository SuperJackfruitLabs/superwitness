<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import { fetched } from "../data/fetched.svelte";
  import { list } from "../format";
  import { fullItems } from "../run/full.svelte";
  import { type Part, requestResponse, runURL, type SeqRange, seqParam, type TranscriptPage, transcriptURL } from "../run/transcript";
  import Empty from "../ui/Empty.svelte";
  import ErrorPanel from "../ui/ErrorPanel.svelte";
  import Loading from "../ui/Loading.svelte";
  import TranscriptRefusal from "./TranscriptRefusal.svelte";
  import Value from "./Value.svelte";

  // SpanSession is a span's session content: what was asked and what came back, read only once the
  // span is opened (the old SpanContent).
  let { api, page, range, spanName, running }: { api: string; page: string; range: SeqRange; spanName: string; running: boolean } = $props();

  const tx = fetched<TranscriptPage>(() => transcriptURL(api, range.attempt, { from: range.from, to: range.to }));
  const full = fullItems(() => api, () => range.attempt, () => ({ from: range.from, to: range.to }));
  const parts = $derived.by(() => {
    if (!tx.data) return null;
    const items = list(tx.data.items).map((it) => full.resolve(it));
    return requestResponse(spanName, items, range.from);
  });
  const redacted = $derived(tx.data?.redactions ?? 0);
</script>

{#snippet values(ps: Part[])}
  {#if ps.length === 0}
    <Empty>none in this span</Empty>
  {:else}
    {#each ps as p, i (i)}<Value v={p.value} onfull={() => full.load(p.item)} />{/each}
  {/if}
{/snippet}

<section class="sw-session" aria-labelledby="sw-session-h">
  <h3 id="sw-session-h">Session content</h3>
  {#if running}<Empty>Attempt still running; refresh for more</Empty>{/if}
  {#if tx.error}
    <TranscriptRefusal error={tx.error} onretry={tx.reload} />
  {:else if !parts}
    <Loading label="Loading session content…" />
  {:else}
    <h4>Request</h4>
    {@render values(parts.request)}
    <h4>Response</h4>
    {@render values(parts.response)}
    <Empty>{redacted} value{redacted === 1 ? "" : "s"} redacted</Empty>
    {#if full.error}<ErrorPanel message={full.error} />{/if}
  {/if}
  {#if tx.data && tx.data.next_cursor !== null}<Empty>More steps than shown here; open the transcript for the rest.</Empty>{/if}
  <p class="sw-session__open">
    <Button variant="secondary" href={runURL(page, { tab: "transcript", attempt: range.attempt, seq: seqParam(range.from, range.to) })}>Open in transcript ↗</Button>
  </p>
</section>

<style>
  .sw-session { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  h3 { margin: 0 0 4px; font: 700 1rem/1.2 var(--vk-font-display); }
  h4 { margin: 6px 0 0; font: 700 0.8125rem/1.3 var(--vk-font-body); color: var(--vk-color-muted); }
  .sw-session__open { margin: 8px 0 0; }
  :global(:where([data-vibe="paper"])) h3 { font: italic 500 22px/1.2 var(--vk-font-display); padding-bottom: 6px; border-bottom: 1px solid var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) h3 { font: 800 15px var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) h3 { font: 700 1.125rem/1.3 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) h4 { color: var(--vk-color-text); font-size: 1rem; }
</style>
