<script lang="ts">
  // Redaction draws text with the hub's redaction markers ("[redacted:anthropic-key]") as marks
  // inside it: the words stay exactly as they came, only the markers are set apart.
  let { text }: { text: string } = $props();
  const parts = $derived(text.split(/(\[redacted:[^\]\s]*\])/));
</script>

{#each parts as p, i (i)}{#if i % 2 === 1}<mark class="sw-redaction">{p}</mark>{:else}{p}{/if}{/each}

<style>
  .sw-redaction {
    padding: 1px 6px; border-radius: 6px; background: var(--vk-color-text); color: var(--vk-color-bg);
    font: 600 0.8125rem/1.6 var(--vk-font-mono); overflow-wrap: anywhere; box-decoration-break: clone; -webkit-box-decoration-break: clone;
  }
  :global(:where([data-vibe="paper"])) .sw-redaction { padding: 0 3px; border-radius: 0; background: var(--sw-warn-wash); color: var(--vk-color-text); box-shadow: inset 0 0 0 1px var(--vk-color-text); font-style: italic; font-weight: 500; }
  :global(:where([data-vibe="studio"])) .sw-redaction { border-radius: 0; font-weight: 700; }
  :global(:where([data-vibe="quiet"])) .sw-redaction { padding: 0 2px; border-radius: 0; background: none; color: var(--vk-color-text); outline: 2px dashed var(--vk-color-text); outline-offset: -1px; font-weight: 400; }
</style>
