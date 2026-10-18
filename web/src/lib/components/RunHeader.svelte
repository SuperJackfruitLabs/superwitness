<script lang="ts">
  import { Button, Sky } from "@superjackfruit/vibekit";
  import { appearance } from "../appearance.svelte";
  import type { Doc } from "../types";
  import StatusPill from "../ui/StatusPill.svelte";
  import Facts from "./Facts.svelte";

  // RunHeader (SW-RunTrace-*): "superpipeline · board/run", the title (board/run when the card's
  // title is unknown), the run's state, Record verdict, then the facts. Daylight sets it all in the
  // sky band; Paper rules it; Studio and Quiet draw it plain.
  let { board, run, doc, onrecord }: { board: string; run: string; doc: Doc; onrecord: () => void } = $props();
  const title = $derived(doc.run?.card?.title && doc.run.card.title !== "unknown" ? String(doc.run.card.title) : `${board}/${run}`);
</script>

{#snippet head()}
  <p class="sw-runhead__ref">superpipeline · {board}/{run}</p>
  <div class="sw-runhead__row">
    <h1>{title}</h1>
    <StatusPill status={doc.run?.state ?? "unknown"} />
    <span class="sw-runhead__act"><Button variant="primary" onclick={onrecord}>Record verdict</Button></span>
  </div>
{/snippet}

{#if appearance.value.vibe === "daylight"}
  <div class="sw-runhead sw-runhead--sky">
    <Sky phase={appearance.phase}>
      {@render head()}
      <Facts {doc} />
    </Sky>
  </div>
{:else}
  <header class="sw-runhead">{@render head()}</header>
  <Facts {doc} />
{/if}

<style>
  .sw-runhead { min-width: 0; }
  .sw-runhead__ref { margin: 0; font: 500 0.8125rem/1.4 var(--vk-font-mono); opacity: 0.85; overflow-wrap: anywhere; }
  .sw-runhead__row { display: flex; align-items: center; flex-wrap: wrap; gap: 14px 18px; margin-top: 10px; }
  h1 { margin: 0; min-width: 0; font: 800 2.75rem/1.05 var(--vk-font-display); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  .sw-runhead__act { margin-left: auto; }

  /* Daylight: the sky band. */
  .sw-runhead--sky :global(.vk-sky) { border-radius: var(--vk-radius-lg); overflow: hidden; }
  .sw-runhead--sky :global(.vk-sky__content) { padding: 24px 104px 28px 32px; } /* right: clear of the sun */
  .sw-runhead--sky :global(.sw-pill) { font-size: var(--vk-text-md); padding: 5px 14px; }

  /* Paper: a top rule, the title in a large serif, state and action at the right. */
  :global(:where([data-vibe="paper"])) .sw-runhead { padding: 20px 0 18px; border-top: 1px solid var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-runhead__ref { font: 400 13px/1.5 var(--vk-font-mono); color: var(--vk-color-muted); opacity: 1; }
  :global(:where([data-vibe="paper"])) .sw-runhead__row { align-items: flex-end; gap: 16px 24px; margin-top: 6px; }
  :global(:where([data-vibe="paper"])) h1 { font: 500 56px/1.02 var(--vk-font-display); }

  /* Studio: a condensed bold title with the state beside it, the action at the right, on a rule. */
  :global(:where([data-vibe="studio"])) .sw-runhead { padding: 24px 0 20px; border-bottom: 1.5px solid var(--vk-color-rule); }
  :global(:where([data-vibe="studio"])) .sw-runhead__ref { font: 500 12.5px var(--vk-font-mono); color: var(--vk-color-muted); opacity: 1; }
  :global(:where([data-vibe="studio"])) .sw-runhead__row { gap: 10px 16px; margin-top: 8px; }
  :global(:where([data-vibe="studio"])) h1 { font: 800 44px/1.02 var(--vk-font-body); font-stretch: 82%; }

  /* Quiet: a muted mono line, a plain title. */
  :global(:where([data-vibe="quiet"])) .sw-runhead { padding-top: var(--vk-space-5, 24px); }
  :global(:where([data-vibe="quiet"])) .sw-runhead__ref { color: var(--vk-color-muted); font-size: 0.9375rem; opacity: 1; }
  :global(:where([data-vibe="quiet"])) .sw-runhead__row { gap: var(--vk-space-4, 16px); margin: var(--vk-space-2, 8px) 0 var(--vk-space-4, 16px); }
  :global(:where([data-vibe="quiet"])) h1 { font: 700 2.25rem/1.2 var(--vk-font-display); letter-spacing: 0; }

  @media (max-width: 759px) {
    .sw-runhead--sky :global(.vk-sky__content) { padding: 16px 16px 20px; }
    /* The Daylight sun sits top right: the title wraps before it rather than running under it. */
    .sw-runhead--sky h1 { padding-right: 56px; }
    h1 { font-size: 1.75rem; }
    .sw-runhead__row { gap: 10px 12px; }
    :global(:where([data-vibe="paper"])) h1 { font-size: 34px; }
    :global(:where([data-vibe="studio"])) h1 { font-size: 30px; }
    :global(:where([data-vibe="quiet"])) h1 { font-size: 1.625rem; }
  }
</style>
