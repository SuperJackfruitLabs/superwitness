<script lang="ts">
  import type { Snippet } from "svelte";
  import { Sky } from "@superjackfruit/vibekit";
  import { appearance } from "../appearance.svelte";

  // PageHead is a page's head: in the Daylight sky band, plain in the other vibes (as the runs list).
  let { children }: { children: Snippet } = $props();
</script>

{#if appearance.value.vibe === "daylight"}
  <div class="sw-pagehead sw-pagehead--sky"><Sky phase={appearance.phase}>{@render children()}</Sky></div>
{:else}
  <header class="sw-pagehead">{@render children()}</header>
{/if}

<style>
  .sw-pagehead { min-width: 0; margin-bottom: 16px; }
  .sw-pagehead :global(h1) { margin: 0; font: 800 2.75rem/1.05 var(--vk-font-display); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  :global(:where([data-vibe="paper"])) .sw-pagehead { padding-bottom: 18px; border-bottom: 1px solid var(--vk-color-text); margin-bottom: 22px; }
  :global(:where([data-vibe="paper"])) .sw-pagehead :global(h1) { font-weight: 500; font-size: 3.25rem; }
  :global(:where([data-vibe="studio"])) .sw-pagehead { padding-bottom: 20px; border-bottom: 1px solid var(--vk-color-text); margin-bottom: 24px; }
  :global(:where([data-vibe="quiet"])) .sw-pagehead :global(h1) { font: 700 2rem/1.2 var(--vk-font-body); }
  @media (max-width: 759px) { .sw-pagehead :global(h1) { font-size: 2rem; } }
</style>
