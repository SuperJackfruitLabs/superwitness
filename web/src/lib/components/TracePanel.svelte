<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import { getJSON } from "../api";
  import { pages } from "../data/pages.svelte";
  import { type Evidence, MAX_EVIDENCE } from "../evidence.svelte";
  import { list } from "../format";
  import { navigate } from "../nav";
  import { waterfall } from "../run/facts";
  import { runURL } from "../run/transcript";
  import type { Doc, Span } from "../types";
  import Empty from "../ui/Empty.svelte";
  import ErrorPanel from "../ui/ErrorPanel.svelte";
  import Loading from "../ui/Loading.svelte";
  import Waterfall from "../ui/Waterfall.svelte";
  import SpanPane from "./SpanPane.svelte";

  // TracePanel is the span waterfall, 500 spans a page. Ticked spans become the verdict's evidence;
  // a span's name opens its details, named in the URL (?span=). A ?span= that names no loaded span
  // opens nothing, as the old app did.
  let { api, page, attempts = [], selected = null, evidence }: { api: string; page: string; attempts?: Doc[]; selected?: string | null; evidence: Evidence } = $props();

  const spans = pages<Span>(
    () => api,
    (cursor) =>
      getJSON<{ spans: Span[]; next_cursor?: string }>(`${api}/spans?limit=500${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`).then((p) => ({
        items: list(p.spans),
        next: p.next_cursor ?? null,
      })),
  );
  const rows = $derived(waterfall(spans.items));
  const total = $derived.by(() => {
    if (spans.items.length === 0) return 0;
    const starts = spans.items.map((s) => Date.parse(s.start));
    return Math.max(...spans.items.map((s, i) => starts[i] + s.duration_ms)) - Math.min(...starts);
  });
  const sel = $derived(selected ? spans.items.find((s) => s.span_id === selected) : undefined);
</script>

{#if spans.error}
  <div class="sw-trace__fail">
    <ErrorPanel message={spans.error.message} />
    <Button variant="secondary" onclick={spans.retry}>Try again</Button>
  </div>
{:else if spans.loading}
  <Loading label="Loading spans…" />
{:else if spans.items.length === 0}
  <Empty>No spans.</Empty>
{:else}
  <p class="sw-trace__intro">
    Tick spans to cite them in a verdict (<b>{evidence.count} of at most {MAX_EVIDENCE}</b>). A span's name opens its details.
  </p>
  <Waterfall
    {rows}
    {total}
    ticked={(id) => evidence.has(id)}
    canAdd={evidence.canAdd}
    selected={sel ? sel.span_id : null}
    ontick={(id) => evidence.toggle(id)}
    onopen={(id) => navigate(runURL(page, { span: id }))}
  />
  {#if spans.moreError}<div class="sw-trace__more-err"><ErrorPanel message={spans.moreError.message} /></div>{/if}
  {#if spans.next}
    <p class="sw-trace__more">
      <Button variant="secondary" onclick={spans.more}>{spans.moreError ? "Try loading more spans again" : "Load more spans"}</Button>
    </p>
  {/if}
  {#if sel}
    {#key sel.span_id}
      <SpanPane span={sel} {api} {page} {attempts} cited={evidence.has(sel.span_id)} canCite={evidence.canAdd} oncite={() => evidence.toggle(sel.span_id)} />
    {/key}
  {/if}
{/if}

<style>
  .sw-trace__intro { margin: 0 0 14px; color: var(--vk-color-muted); }
  .sw-trace__intro b { color: var(--vk-color-text); }
  .sw-trace__fail { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
  .sw-trace__more-err { margin-top: 12px; }
  .sw-trace__more { display: flex; justify-content: center; margin: 16px 0 0; }
  :global(:where([data-vibe="paper"])) .sw-trace__intro { margin: 0 0 12px; font: italic 400 18px/1.45 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-trace__intro b { font: normal 600 16px var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-trace__intro { color: var(--vk-color-text); font: 400 16px/1.45 var(--vk-font-serif); }
  :global(:where([data-vibe="studio"])) .sw-trace__intro b { font-weight: 400; }
  :global(:where([data-vibe="quiet"])) .sw-trace__intro { color: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-trace__intro b { font-weight: 400; }
  @media (max-width: 759px) {
    .sw-trace__more :global(.vk-button) { width: 100%; }
  }
</style>
