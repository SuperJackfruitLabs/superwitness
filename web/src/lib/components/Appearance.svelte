<script lang="ts">
  import { accentOptions, VIBES, type Vibe } from "@superjackfruit/vibekit";
  import { appearance } from "../appearance.svelte";

  // The Appearance control (mockup "Appearance open"): vibe, theme (follow the sun, light, dark)
  // and accent. Every press writes the cookie and <html> through the appearance store.
  const accents = accentOptions("superwitness");
  // vibekit's picker labels (its model does not export them from the package root).
  const VIBE_LABELS: Record<Vibe, string> = { daylight: "Daylight", paper: "Paper", studio: "Studio", quiet: "Quiet" };
  const THEME_OPTIONS = [
    { value: "sun", label: "Follow the sun" },
    { value: "light", label: "Light" },
    { value: "dark", label: "Dark" },
  ] as const;
  const chosen = $derived(accents.find((o) => o.key === appearance.accent) ?? accents[0]);
</script>

<div class="sw-ap" role="group" aria-label="Appearance">
  <fieldset>
    <legend>Vibe</legend>
    <div class="sw-segs">
      {#each VIBES as v (v)}
        <button type="button" aria-pressed={appearance.value.vibe === v} onclick={() => appearance.set({ vibe: v })}>{VIBE_LABELS[v]}</button>
      {/each}
    </div>
  </fieldset>
  <fieldset>
    <legend>Theme</legend>
    <div class="sw-segs sw-segs--3">
      {#each THEME_OPTIONS as t (t.value)}
        <button type="button" aria-pressed={appearance.value.theme === t.value} onclick={() => appearance.set({ theme: t.value })}>{t.label}</button>
      {/each}
    </div>
  </fieldset>
  <fieldset>
    <legend>Accent</legend>
    <div class="sw-sw">
      {#each accents as o (o.key)}
        <button
          type="button"
          class="sw-swatch"
          aria-pressed={appearance.accent === o.key}
          aria-label={o.name}
          title={o.name}
          data-vibe={appearance.value.vibe}
          data-theme={appearance.theme}
          data-accent={o.key}
          data-product="superwitness"
          onclick={() => appearance.setAccent(o.key)}
        ></button>
      {/each}
    </div>
    <p class="sw-hint">{chosen.name}</p>
  </fieldset>
</div>

<style>
  .sw-ap { display: flex; flex-direction: column; gap: 14px; padding: 14px; }
  fieldset { margin: 0; padding: 0; border: 0; min-width: 0; }
  legend { padding: 0; margin-bottom: 8px; font-weight: 700; }
  .sw-segs { display: grid; grid-template-columns: repeat(2, 1fr); gap: 6px; }
  .sw-segs--3 { grid-template-columns: repeat(3, 1fr); }
  .sw-segs button { min-height: 44px; padding: 0 8px; border: 0; border-radius: var(--vk-radius-pill); background: var(--vk-color-raised); color: var(--vk-color-text); font: inherit; font-weight: 600; line-height: 1.15; cursor: pointer; box-shadow: inset 0 0 0 2px var(--vk-color-line); }
  .sw-segs button[aria-pressed="true"] { background: var(--vk-color-accent-wash); color: var(--vk-color-accent-text); font-weight: 800; box-shadow: inset 0 0 0 2px var(--vk-color-accent); }
  .sw-sw { display: flex; flex-wrap: wrap; gap: 8px; }
  .sw-swatch { width: 44px; height: 44px; padding: 0; border: 0; border-radius: 50%; background: var(--vk-color-accent); cursor: pointer; box-shadow: inset 0 0 0 2px var(--vk-color-line); }
  .sw-swatch[aria-pressed="true"] { box-shadow: 0 0 0 3px var(--vk-color-bg, #fff), 0 0 0 5px var(--vk-color-text); }
  .sw-hint { margin: 6px 0 0; font-size: var(--vk-text-sm); color: var(--vk-color-muted); }

  :global(:where([data-vibe="paper"])) .sw-segs button { border-radius: var(--vk-radius-sm); background: transparent; box-shadow: inset 0 0 0 1px var(--vk-color-line); font-weight: 500; }
  :global(:where([data-vibe="paper"])) .sw-segs button[aria-pressed="true"] { font-style: italic; box-shadow: inset 0 0 0 1px var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) legend { font: 600 15px/1 var(--vk-font-body); font-variant-caps: all-small-caps; letter-spacing: 0.12em; color: var(--vk-color-muted); }
  :global(:where([data-vibe="studio"])) .sw-segs { gap: 0; }
  :global(:where([data-vibe="studio"])) .sw-segs button { border-radius: 0; background: transparent; box-shadow: inset 0 0 0 1px var(--vk-color-rule); font-size: 13px; }
  :global(:where([data-vibe="studio"])) .sw-segs button[aria-pressed="true"] { background: var(--vk-color-text); color: var(--vk-color-bg); box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-swatch { border-radius: 0; width: 36px; height: 36px; }
  :global(:where([data-vibe="studio"])) legend { font: 700 11px/1 var(--vk-font-body); letter-spacing: 0.12em; text-transform: uppercase; }
  :global(:where([data-vibe="quiet"])) .sw-segs button { border-radius: var(--vk-radius-sm); background: transparent; box-shadow: inset 0 0 0 1px var(--vk-color-line); font-weight: 400; }
  :global(:where([data-vibe="quiet"])) .sw-segs button[aria-pressed="true"] { font-weight: 700; background: var(--vk-color-accent-wash); box-shadow: inset 0 0 0 2px var(--vk-color-accent); }
</style>
