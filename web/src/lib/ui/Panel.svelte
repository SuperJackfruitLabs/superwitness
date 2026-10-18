<script lang="ts">
  import type { Snippet } from "svelte";
  import Mark from "./Mark.svelte";
  // Panel is the focus layout of the edge pages (SW-SignedOut, SW-Edge, SW-Phone-Edge): no shell,
  // one centred column, the superwitness name above. Daylight draws a raised card with the mark
  // inside; Paper a nameplate over a double rule; Studio the name, then a ruled panel; Quiet a
  // header line and plain text.
  let { children }: { children: Snippet } = $props();
</script>

<main class="sw-focus">
  <div class="sw-focus__brand"><Mark size={16} /><span>superwitness</span></div>
  <span class="sw-focus__rule" aria-hidden="true"></span>
  <section class="sw-focus__col">
    <span class="sw-focus__mark"><Mark size={32} /></span>
    {@render children()}
  </section>
</main>

<style>
  .sw-focus { box-sizing: border-box; width: 100%; max-width: 560px; margin: 0 auto; padding: 72px 16px 56px; overflow-wrap: anywhere; }
  .sw-focus__brand { display: flex; align-items: center; gap: 10px; min-height: 44px; font-weight: 700; }
  .sw-focus__rule { display: none; }
  .sw-focus__mark { display: none; }
  .sw-focus__col { display: flex; flex-direction: column; align-items: flex-start; gap: 20px; }
  .sw-focus__col :global(h1) { margin: 0; }
  .sw-focus__col :global(p) { margin: 0; }

  /* Daylight: the card, centred, the mark tile inside it. */
  :global(:where([data-vibe="daylight"])) .sw-focus { max-width: 460px; padding-top: 12vh; }
  :global(:where([data-vibe="daylight"])) .sw-focus__brand { display: none; }
  :global(:where([data-vibe="daylight"])) .sw-focus__col { align-items: center; padding: 40px 36px 36px; border-radius: var(--vk-radius-lg); background: var(--vk-color-surface); box-shadow: 0 18px 48px -20px var(--vk-color-ground), 0 var(--vk-lift) 0 var(--vk-color-ground); text-align: center; }
  :global(:where([data-vibe="daylight"])) .sw-focus__mark { display: block; }
  :global(:where([data-vibe="daylight"])) .sw-focus__mark :global(.sw-mark) { width: 64px; height: 64px; border-radius: var(--vk-radius-lg); }
  :global(:where([data-vibe="daylight"])) .sw-focus__col :global(h1) { font: 800 2.75rem/1 var(--vk-font-display); letter-spacing: -0.02em; }
  :global(:where([data-vibe="daylight"])) .sw-focus__col :global(.sw-lead) { font-size: 1.125rem; color: var(--vk-color-muted); max-width: 32ch; }
  :global(:where([data-vibe="daylight"])) .sw-focus__col :global(.vk-button) { min-width: 200px; min-height: 48px; justify-content: center; }

  /* Paper: nameplate in small capitals over a double rule, a large serif title. */
  :global(:where([data-vibe="paper"])) .sw-focus { padding-top: 48px; text-align: center; }
  :global(:where([data-vibe="paper"])) .sw-focus__brand { justify-content: center; min-height: 48px; font: 600 22px/1 var(--vk-font-display); font-variant-caps: all-small-caps; letter-spacing: 0.06em; }
  :global(:where([data-vibe="paper"])) .sw-focus__rule { display: block; margin-top: 14px; border-top: 1px solid var(--vk-color-text); border-bottom: 3px double var(--vk-color-text); height: 6px; }
  :global(:where([data-vibe="paper"])) .sw-focus__col { align-items: center; gap: 0; }
  :global(:where([data-vibe="paper"])) .sw-focus__col :global(h1) { margin-top: 44px; font: 500 clamp(44px, 10vw, 76px)/1.05 var(--vk-font-display); letter-spacing: -0.02em; }
  :global(:where([data-vibe="paper"])) .sw-focus__col :global(h1 + *) { margin-top: 16px; }
  :global(:where([data-vibe="paper"])) .sw-focus__col :global(.sw-lead) { font: italic 400 22px/1.45 var(--vk-font-display); color: var(--vk-color-muted); max-width: 46ch; }
  :global(:where([data-vibe="paper"])) .sw-focus__col :global(.sw-act) { margin-top: 32px; }

  /* Studio: the name, then a panel under a 1.5px rule, condensed heavy title, serif lede. */
  :global(:where([data-vibe="studio"])) .sw-focus { max-width: 520px; }
  :global(:where([data-vibe="studio"])) .sw-focus__brand { font: 800 20px/1 var(--vk-font-body); font-stretch: 85%; margin-bottom: 24px; }
  :global(:where([data-vibe="studio"])) .sw-focus__col { padding-top: 18px; border-top: 1.5px solid var(--vk-color-rule); }
  :global(:where([data-vibe="studio"])) .sw-focus__col :global(h1) { font: 900 clamp(48px, 14vw, 96px)/0.86 var(--vk-font-body); font-stretch: 72%; letter-spacing: -0.04em; }
  :global(:where([data-vibe="studio"])) .sw-focus__col :global(.sw-lead) { font: 400 20px/1.45 var(--vk-font-serif); }

  /* Quiet: a header line with a small square and the name, then plain text. */
  :global(:where([data-vibe="quiet"])) .sw-focus { max-width: 640px; padding: 24px 16px 56px; font-size: 1.125rem; line-height: 1.6; }
  :global(:where([data-vibe="quiet"])) .sw-focus__brand { padding-bottom: 12px; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="quiet"])) .sw-focus__col { gap: 16px; }
  :global(:where([data-vibe="quiet"])) .sw-focus__col :global(h1) { margin-top: 48px; font: 700 2.25rem/1.2 var(--vk-font-display); }
  :global(:where([data-vibe="quiet"])) .sw-focus__col :global(.sw-act), :global(:where([data-vibe="quiet"])) .sw-focus__col :global(.sw-act .vk-button) { width: 100%; }
</style>
