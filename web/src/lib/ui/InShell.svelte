<script lang="ts">
  import type { Snippet } from "svelte";
  // InShell is an edge state drawn inside the shell (run, rubric or page not found): a heading,
  // a muted line, a way back. No frame of its own: the shell is the frame.
  let { children }: { children: Snippet } = $props();
</script>

<section class="sw-inshell">{@render children()}</section>

<style>
  .sw-inshell { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; padding: 36px 28px; overflow-wrap: anywhere; }
  .sw-inshell :global(h1) { margin: 0; font: 800 1.75rem/1.1 var(--vk-font-display); }
  .sw-inshell :global(p) { margin: 0; color: var(--vk-color-muted); }
  :global(:where([data-vibe="daylight"])) .sw-inshell { border-radius: var(--vk-radius-md); background: var(--vk-color-surface); }
  :global(:where([data-vibe="paper"])) .sw-inshell { padding: 22px 0 16px; }
  :global(:where([data-vibe="paper"])) .sw-inshell :global(h1) { width: 100%; padding: 22px 0 16px; border-top: 1px solid var(--vk-color-text); border-bottom: 2px solid var(--vk-color-text); font: 500 56px/1 var(--vk-font-display); letter-spacing: -0.02em; }
  :global(:where([data-vibe="paper"])) .sw-inshell :global(p) { font: italic 400 21px/1.45 var(--vk-font-display); }
  :global(:where([data-vibe="studio"])) .sw-inshell { gap: 16px; padding: 28px 0; }
  :global(:where([data-vibe="studio"])) .sw-inshell :global(h1) { width: 100%; padding-bottom: 14px; border-bottom: 1.5px solid var(--vk-color-rule); font: 800 40px/1 var(--vk-font-body); font-stretch: 80%; letter-spacing: -0.02em; }
  :global(:where([data-vibe="studio"])) .sw-inshell :global(p) { font: 400 19px/1.45 var(--vk-font-serif); }
  :global(:where([data-vibe="quiet"])) .sw-inshell { padding: 0; }
  :global(:where([data-vibe="quiet"])) .sw-inshell :global(h1) { margin-top: 32px; font: 700 1.75rem/1.25 var(--vk-font-display); }
</style>
