<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import { hasTruncated, showValue } from "../run/transcript";
  import Empty from "../ui/Empty.svelte";
  import Redaction from "../ui/Redaction.svelte";
  // Value is one transcript field as text, wrapping inside its block; a field the hub cut offers
  // Show full.
  // prose: a person's or the agent's words (prompt, message) in the body face; else monospace.
  let { v, onfull, prose = false }: { v: unknown; onfull?: () => void; prose?: boolean } = $props();
</script>

{#if v === undefined || v === null}
  <Empty>none</Empty>
{:else}
  <pre class="sw-value" class:prose><Redaction text={showValue(v)} /></pre>
  {#if onfull && hasTruncated(v)}<Button variant="secondary" onclick={onfull}>Show full</Button>{/if}
{/if}

<style>
  .sw-value {
    margin: 4px 0 8px; padding: 10px 12px; border-radius: var(--vk-radius-sm); background: var(--vk-color-raised);
    font: 500 0.8125rem/1.5 var(--vk-font-mono); white-space: pre-wrap; overflow-wrap: anywhere;
  }
  .sw-value.prose { padding: 0; background: none; font: 400 1rem/1.55 var(--vk-font-body); }
  :global(:where([data-vibe="paper"])) .sw-value.prose { border: 0; font: 400 1.0625rem/1.55 var(--vk-font-display); }
  :global(:where([data-vibe="studio"])) .sw-value.prose { box-shadow: none; background: none; font: 400 16px/1.6 var(--vk-font-serif); }
  :global(:where([data-vibe="paper"])) .sw-value { border-radius: 0; background: none; border-left: 2px solid var(--vk-color-line); }
  :global(:where([data-vibe="studio"])) .sw-value { border-radius: 0; background: var(--vk-color-surface); box-shadow: inset 0 0 0 1px var(--vk-color-line); font-size: 14px; }
  :global(:where([data-vibe="quiet"])) .sw-value { border-radius: 0; font-size: 1rem; }
</style>
