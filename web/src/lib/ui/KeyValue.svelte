<script lang="ts" module>
  import type { Snippet } from "svelte";
  export interface KV {
    term: string;
    value: string | Snippet;
    // wide spans the whole row (Daylight's cards); mono sets the value in the mono face.
    wide?: boolean;
    mono?: boolean;
  }
</script>

<script lang="ts">
  // KeyValue is a short list of named values, one wrapper per pair so each vibe can draw it as cards
  // (Daylight), a ruled table (Paper, Quiet) or a row of columns (Studio). vibekit FactList draws the
  // pairs flat, without the wrapper.
  let { items }: { items: KV[] } = $props();
</script>

<dl class="sw-kv">
  {#each items as it (it.term)}
    <div class:wide={it.wide}>
      <dt>{it.term}</dt>
      <dd class:mono={it.mono}>{#if typeof it.value === "string"}{it.value}{:else}{@render it.value()}{/if}</dd>
    </div>
  {/each}
</dl>

<style>
  .sw-kv { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin: 0; }
  .sw-kv > div { min-width: 0; padding: 8px 12px; border-radius: var(--vk-radius-sm); background: var(--vk-color-raised); }
  .sw-kv > div.wide { grid-column: 1 / -1; }
  dt { font-size: 0.75rem; font-weight: 600; color: var(--vk-color-muted); }
  dd { margin: 0; font-weight: 700; overflow-wrap: anywhere; }
  dd.mono { font-family: var(--vk-font-mono); font-weight: 500; }
  :global(:where([data-vibe="paper"])) .sw-kv,
  :global(:where([data-vibe="quiet"])) .sw-kv { grid-template-columns: minmax(0, 1fr); gap: 0; }
  :global(:where([data-vibe="paper"])) .sw-kv > div,
  :global(:where([data-vibe="quiet"])) .sw-kv > div { display: grid; grid-template-columns: 86px minmax(0, 1fr); gap: 12px; align-items: baseline; padding: 8px 0; border-radius: 0; background: none; }
  :global(:where([data-vibe="paper"])) .sw-kv > div { border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) dt { font: 400 13px/1 var(--vk-font-body); font-variant-caps: all-small-caps; letter-spacing: 0.12em; }
  :global(:where([data-vibe="paper"])) dd { font: 400 17px/1.3 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) dt { font-size: 1rem; font-weight: 400; }
  :global(:where([data-vibe="quiet"])) dd { font-size: 1.0625rem; }
  :global(:where([data-vibe="studio"])) .sw-kv { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 0 16px; }
  :global(:where([data-vibe="studio"])) .sw-kv > div, :global(:where([data-vibe="studio"])) .sw-kv > div.wide { grid-column: auto; padding: 0; border-radius: 0; background: none; }
  :global(:where([data-vibe="studio"])) dt { font: 700 12px var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) dd { font: 500 15px/1.3 var(--vk-font-mono); }
  @media (max-width: 759px) {
    :global(:where([data-vibe="studio"])) .sw-kv { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 16px; }
  }
</style>
