<script lang="ts">
  import type { Snippet } from "svelte";
  // Empty is a muted line for "nothing here yet" inside a page; callers pass the words.
  let { children }: { children: Snippet } = $props();
</script>

<p class="sw-empty">{@render children()}</p>

<style>
  .sw-empty { margin: 0; color: var(--vk-color-muted); }
  :global(:where([data-vibe="paper"])) .sw-empty { font: italic 400 19px/1.45 var(--vk-font-display); }
</style>
