<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import { hasTruncated, showValue } from "../run/transcript";
  import Empty from "../ui/Empty.svelte";
  // Value is one transcript field as text, wrapping inside its block; a field the hub cut offers
  // Show full.
  let { v, onfull }: { v: unknown; onfull?: () => void } = $props();
</script>

{#if v === undefined || v === null}
  <Empty>none</Empty>
{:else}
  <pre class="sw-value">{showValue(v)}</pre>
  {#if onfull && hasTruncated(v)}<Button variant="secondary" onclick={onfull}>Show full</Button>{/if}
{/if}

<style>
  .sw-value {
    margin: 4px 0 8px; padding: 10px 12px; border-radius: var(--vk-radius-sm); background: var(--vk-color-raised);
    font: 500 0.8125rem/1.5 var(--vk-font-mono); white-space: pre-wrap; overflow-wrap: anywhere;
  }
  :global(:where([data-vibe="paper"])) .sw-value { border-radius: 0; background: none; border-left: 2px solid var(--vk-color-line); }
  :global(:where([data-vibe="studio"])) .sw-value { border-radius: 0; background: var(--vk-color-surface); box-shadow: inset 0 0 0 1px var(--vk-color-line); font-size: 14px; }
  :global(:where([data-vibe="quiet"])) .sw-value { border-radius: 0; font-size: 1rem; }
</style>
