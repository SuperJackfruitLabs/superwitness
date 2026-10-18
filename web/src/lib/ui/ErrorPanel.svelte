<script lang="ts">
  // ErrorPanel is the refusal line of an edge page: red on a wash in Daylight, "✕" and red in
  // Paper, an ink bar on the left in Studio and Quiet (their danger is ink).
  let { message, code = false }: { message: string; code?: boolean } = $props();
</script>

<p class="sw-refusal" role="alert">{#if code}<code>{message}</code>{:else}{message}{/if}</p>

<style>
  .sw-refusal { margin: 0; max-width: 46ch; font-weight: 600; overflow-wrap: anywhere; color: var(--sw-danger-text); }
  .sw-refusal code { font-family: var(--vk-font-mono); }
  :global(:where([data-vibe="daylight"])) .sw-refusal { padding: 8px 12px; border-radius: var(--vk-radius-md); background: var(--sw-danger-wash); }
  :global(:where([data-vibe="paper"])) .sw-refusal { margin-inline: auto; }
  :global(:where([data-vibe="paper"])) .sw-refusal::before { content: "✕ "; }
  :global(:where([data-vibe="paper"])) .sw-refusal code { font: 400 14px/1 var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-refusal { padding: 10px 12px; border-left: 6px solid var(--vk-color-text); background: var(--vk-color-surface); color: var(--vk-color-text); font: 700 15px/1.4 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-refusal { padding: 4px 0 4px 14px; border-left: 6px solid var(--vk-color-text); color: var(--vk-color-text); font-weight: 700; }
</style>
