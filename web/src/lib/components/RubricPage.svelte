<script lang="ts">
  import { fetched } from "../data/fetched.svelte";
  import { describeScale } from "../rubrics";
  import type { Rubric } from "../types";
  import Loading from "../ui/Loading.svelte";
  import EdgePage from "./EdgePage.svelte";
  import PageHead from "./PageHead.svelte";

  // One rubric (SW-Rubric-*): Rubrics · standard, the name, who made it and when, the scale in words
  // and as sent, then the rubric's body as written. An unknown one is "Rubric not found".
  let { id, version }: { id: string; version: number } = $props();
  const r = fetched<Rubric>(() => `/v1/rubrics/${encodeURIComponent(id)}/${version}`);
</script>

{#if r.error}
  <EdgePage error={r.error} next={`/rubrics/${id}/${version}`} retry={r.reload} />
{:else if !r.data}
  <Loading />
{:else}
  {@const x = r.data}
  <article class="sw-rubric">
    <PageHead>
      <p class="sw-rubric__ref"><a href="/rubrics">Rubrics</a> · {x.standard}</p>
      <h1>{x.name}</h1>
      <p class="sw-rubric__by">Created {x.created_at.slice(0, 10)} by <span class="mono">{x.created_by}</span></p>
    </PageHead>
    <div class="sw-rubric__panel">
      <h2>Scale</h2>
      <p class="sw-rubric__scale">{describeScale(x.recognised_scale)}</p>
      <pre class="mono">{JSON.stringify(x.scale)}</pre>
      <h2>Rubric</h2>
      <pre class="sw-rubric__body">{x.body}</pre>
    </div>
  </article>
{/if}

<style>
  .sw-rubric { min-width: 0; }
  .sw-rubric__ref { margin: 0 0 8px; font: 500 0.8125rem/1.4 var(--vk-font-mono); overflow-wrap: anywhere; }
  .sw-rubric__ref a { color: inherit; font-weight: 700; }
  .sw-rubric__by { margin: 10px 0 0; }
  .mono { font-family: var(--vk-font-mono); }
  .sw-rubric__panel { padding: 20px 24px 24px; border-radius: var(--vk-radius-lg); background: var(--vk-color-surface); box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); }
  h2 { margin: 0 0 6px; font: 800 1.375rem/1.2 var(--vk-font-display); }
  pre + h2 { margin-top: 24px; }
  .sw-rubric__scale { margin: 0 0 12px; font-weight: 600; }
  pre { margin: 0; padding: 12px 14px; border-radius: var(--vk-radius-sm); background: var(--vk-color-raised); white-space: pre-wrap; overflow-wrap: anywhere; font-size: 0.9375rem; }
  .sw-rubric__body { font-family: var(--vk-font-body); line-height: 1.55; }
  :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .sw-rubric__panel { padding: 0; border-radius: 0; background: none; box-shadow: none; }
  :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) pre { border-radius: 0; background: none; padding: 12px 0; border-block: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) h2 { font-weight: 500; }
  :global(:where([data-vibe="paper"])) .sw-rubric__body { font: 400 1.125rem/1.6 var(--vk-font-display); }
  :global(:where([data-vibe="quiet"])) h2 { font: 700 1.25rem/1.3 var(--vk-font-body); }
  @media (max-width: 759px) { .sw-rubric__panel { padding: 16px; } }
</style>
