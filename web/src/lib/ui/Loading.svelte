<script lang="ts">
  // The words "Loading…" are always there (screen readers, tests). Below them each vibe draws its
  // placeholder (SW-RunsStates): Daylight soft bars, Paper ruled blank lines, Studio grey blocks
  // under "— LOADING…", Quiet nothing. bare is the app-level loading before the shell exists.
  let { bare = false }: { bare?: boolean } = $props();
</script>

{#if bare}
  <p class="sw-loading sw-loading--bare" role="status">Loading…</p>
{:else}
  <div class="sw-loading" role="status">
    <p class="sw-loading__t"><span class="sw-loading__dash" aria-hidden="true">— </span>Loading…</p>
    <div class="sw-loading__bars" aria-hidden="true"><span></span><span></span><span></span><span></span></div>
  </div>
{/if}

<style>
  .sw-loading { margin-top: 16px; display: flex; flex-direction: column; gap: 10px; }
  .sw-loading--bare { margin: 0; padding: 24px; font-weight: 600; color: var(--vk-color-muted); }
  .sw-loading__t { margin: 0; font-weight: 600; color: var(--vk-color-muted); }
  .sw-loading__dash { display: none; }
  .sw-loading__bars { display: flex; flex-direction: column; gap: 10px; }
  .sw-loading__bars span { display: block; }
  /* Daylight: three soft bars, the last fainter. */
  :global(:where([data-vibe="daylight"])) .sw-loading__bars span { height: 60px; border-radius: var(--vk-radius-md); background: linear-gradient(90deg, var(--vk-color-surface), var(--vk-color-raised), var(--vk-color-surface)); box-shadow: 0 var(--vk-lift) 0 var(--vk-color-ground); }
  :global(:where([data-vibe="daylight"])) .sw-loading__bars span:nth-child(4) { display: none; }
  :global(:where([data-vibe="daylight"])) .sw-loading__bars span:nth-child(3) { opacity: 0.6; }
  /* Paper: italic words, then ruled blank lines. */
  :global(:where([data-vibe="paper"])) .sw-loading { margin-top: 18px; }
  :global(:where([data-vibe="paper"])) .sw-loading__t { font: italic 400 18px/1.4 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-loading__bars { gap: 0; }
  :global(:where([data-vibe="paper"])) .sw-loading__bars span { height: 52px; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-loading__bars span:first-child { border-top: 1px solid var(--vk-color-text); }
  /* Studio: a rule, "— LOADING…", grey blocks. */
  :global(:where([data-vibe="studio"])) .sw-loading { border-top: 1.5px solid var(--vk-color-rule); padding-top: 12px; }
  :global(:where([data-vibe="studio"])) .sw-loading__t { font: 700 12px/1 var(--vk-font-body); letter-spacing: 0.12em; text-transform: uppercase; }
  :global(:where([data-vibe="studio"])) .sw-loading__dash { display: inline; }
  :global(:where([data-vibe="studio"])) .sw-loading__bars span { height: 18px; background: var(--vk-color-line); }
  :global(:where([data-vibe="studio"])) .sw-loading__bars span:nth-child(1) { width: 92%; }
  :global(:where([data-vibe="studio"])) .sw-loading__bars span:nth-child(2) { width: 78%; }
  :global(:where([data-vibe="studio"])) .sw-loading__bars span:nth-child(3) { width: 86%; }
  :global(:where([data-vibe="studio"])) .sw-loading__bars span:nth-child(4) { width: 64%; }
  /* Quiet: plain words. */
  :global(:where([data-vibe="quiet"])) .sw-loading__bars { display: none; }
  :global(:where([data-vibe="quiet"])) .sw-loading__t { font-weight: 400; }
  :global(:where([data-vibe="paper"])) .sw-loading--bare { font: italic 400 19px/1.4 var(--vk-font-display); }
  :global(:where([data-vibe="studio"])) .sw-loading--bare { font: 500 14px var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-loading--bare { font-weight: 400; }
</style>
