<script lang="ts">
  import { facts } from "../run/facts";
  import type { Doc } from "../types";

  // Facts is the strip under a run's title: Agent, Attempts, Errors, Duration, Cost. A value a
  // source could not supply is a word ("unknown", "in progress", "unreported"), drawn muted and
  // italic, never blank or 0. The agent is a principal id: mono, and free to wrap inside its fact.
  let { doc }: { doc: Doc } = $props();
  const WORDS = new Set(["unknown", "in progress", "unreported"]);
  const items = $derived(facts(doc));
</script>

<dl class="sw-facts">
  {#each items as f (f.label)}
    <div>
      <dt>{f.label}</dt>
      <dd class:sw-word={WORDS.has(f.value)} class:sw-mono={f.label === "Agent" && !WORDS.has(f.value)}>{f.value}</dd>
    </div>
  {/each}
</dl>

<style>
  /* Daylight (base): five white cards in the sky band; three and two beside an open span pane. */
  .sw-facts { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 10px; margin: 20px 0 0; }
  .sw-facts > div { min-width: 0; padding: 12px 16px; border-radius: var(--vk-radius-md); background: var(--vk-color-surface); color: var(--vk-color-text); }
  dt { font-size: var(--vk-text-sm); font-weight: 600; color: var(--vk-color-muted); }
  dd { margin: 2px 0 0; font: 700 1.125rem/1.3 var(--vk-font-display); overflow-wrap: break-word; }
  dd.sw-mono { font: 500 0.9375rem/1.4 var(--vk-font-mono); }
  dd.sw-word { color: var(--vk-color-muted); font-style: italic; font-weight: 600; }

  /* Paper: a ruled ledger row, serif figures, small-caps terms. */
  :global(:where([data-vibe="paper"])) .sw-facts { gap: 0; margin: 0; border-top: 2px solid var(--vk-color-text); border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-facts > div { padding: 12px 18px 14px; border-radius: 0; background: none; }
  :global(:where([data-vibe="paper"])) .sw-facts > div:first-child { padding-left: 0; }
  :global(:where([data-vibe="paper"])) .sw-facts > div + div { border-left: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) dt { font-weight: 400; font-size: 15px; font-variant-caps: all-small-caps; letter-spacing: 0.12em; }
  :global(:where([data-vibe="paper"])) dd { font: 500 28px/1.2 var(--vk-font-display); font-variant-numeric: tabular-nums; }
  :global(:where([data-vibe="paper"])) dd.sw-mono { font: 400 16px/1.5 var(--vk-font-mono); padding-top: 7px; }
  :global(:where([data-vibe="paper"])) dd.sw-word { font-style: italic; font-weight: 500; }

  /* Studio: mono figures between hairlines. */
  :global(:where([data-vibe="studio"])) .sw-facts { gap: 0; margin: 0; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="studio"])) .sw-facts > div { display: flex; flex-direction: column; gap: 6px; margin-right: 16px; padding: 14px 16px 16px 0; border-radius: 0; background: none; border-right: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="studio"])) .sw-facts > div:last-child { border-right: 0; margin-right: 0; }
  :global(:where([data-vibe="studio"])) dt { font: 700 12px var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) dd { margin: 0; font: 500 34px/1 var(--vk-font-mono); letter-spacing: -0.02em; }
  :global(:where([data-vibe="studio"])) dd.sw-mono { font-size: 22px; line-height: 1.2; letter-spacing: 0; }
  :global(:where([data-vibe="studio"])) dd.sw-word { font: italic 400 26px/1.1 var(--vk-font-serif); }

  /* Quiet: plain terms and bold values between two hairlines. */
  :global(:where([data-vibe="quiet"])) .sw-facts { gap: 4px 24px; margin: 0; padding: var(--vk-space-3, 12px) 0; border-top: 1px solid var(--vk-color-line); border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="quiet"])) .sw-facts > div { padding: 0; border-radius: 0; background: none; }
  :global(:where([data-vibe="quiet"])) dt { font-size: 1rem; font-weight: 400; }
  :global(:where([data-vibe="quiet"])) dd { margin: 0; font: 700 1.125rem/1.6 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) dd.sw-mono { font: 700 0.9375rem/1.9 var(--vk-font-mono); }
  :global(:where([data-vibe="quiet"])) dd.sw-word { color: var(--vk-color-text); font-weight: 700; }

  /* Phone: a wrapping two-column grid, the agent across both columns. */
  @media (max-width: 759px) {
    .sw-facts { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .sw-facts > div:first-child { grid-column: 1 / -1; }
    :global(:where([data-vibe="paper"])) .sw-facts > div,
    :global(:where([data-vibe="paper"])) .sw-facts > div:first-child { padding: 10px 12px 12px 0; border-left: 0; border-bottom: 1px solid var(--vk-color-line); }
    :global(:where([data-vibe="paper"])) .sw-facts > div:nth-child(odd):not(:first-child) { padding-left: 12px; border-left: 1px solid var(--vk-color-line); }
    :global(:where([data-vibe="paper"])) dd { font-size: 22px; }
    :global(:where([data-vibe="studio"])) .sw-facts > div { margin-right: 0; padding: 10px 12px 12px 0; border-right: 0; border-bottom: 1px solid var(--vk-color-line); }
    :global(:where([data-vibe="studio"])) .sw-facts > div:nth-child(odd):not(:first-child) { padding-left: 12px; border-left: 1px solid var(--vk-color-line); }
    :global(:where([data-vibe="studio"])) dd { font-size: 26px; }
    :global(:where([data-vibe="studio"])) dd.sw-word { font-size: 20px; }
  }
</style>
