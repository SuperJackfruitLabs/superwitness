<script lang="ts" module>
  export type PillTone = "success" | "danger" | "warning" | "info" | "neutral";
  // A run status's tone. The word is always drawn too: the tone never carries the meaning alone.
  export function toneOf(status: string): PillTone {
    switch (status) {
      case "succeeded":
      case "completed":
        return "success";
      case "failed":
        return "danger";
      case "waiting":
        return "warning";
      case "running":
        return "info";
    }
    return "neutral";
  }
</script>

<script lang="ts">
  // StatusPill (SW-Runs-*): Daylight a wash pill with a dot, Paper a small-caps ruled word with a
  // square, Studio a mono label with a drawn square (failed is ink), Quiet a bordered word whose
  // border style carries the tone (double for failed, dashed for waiting).
  let { status }: { status: string } = $props();
  const word = $derived(status || "unknown");
  const tone = $derived(toneOf(status));
</script>

<span class="sw-pill" data-tone={tone}><i class="sw-pill__mark" aria-hidden="true"></i>{word}</span>

<style>
  .sw-pill {
    --tw: var(--vk-color-raised); --tt: var(--vk-color-muted); --td: var(--vk-color-faint);
    display: inline-flex; align-items: center; gap: 6px; flex: none; white-space: nowrap;
    padding: 3px 11px; border-radius: var(--vk-radius-pill);
    font: 700 var(--vk-text-sm) / 1.3 var(--vk-font-body);
    background: var(--tw); color: var(--tt); box-shadow: inset 0 0 0 1px var(--td);
  }
  .sw-pill__mark { width: 8px; height: 8px; flex: none; border-radius: 50%; background: var(--td); }
  .sw-pill[data-tone="success"] { --tw: var(--vk-color-accent-wash); --tt: var(--vk-color-accent-text); --td: var(--vk-color-accent); }
  .sw-pill[data-tone="danger"] { --tw: var(--sw-danger-wash); --tt: var(--sw-danger-text); --td: var(--sw-danger); }
  .sw-pill[data-tone="warning"] { --tw: var(--sw-warn-wash); --tt: var(--sw-warn-text); --td: var(--sw-warn); }
  .sw-pill[data-tone="info"] { --tw: var(--sw-info-wash); --tt: var(--sw-info-text); --td: var(--sw-info); }

  /* Paper: small caps, a 1px rule in the tone, a square. */
  :global(:where([data-vibe="paper"])) .sw-pill {
    gap: 7px; padding: 2px 9px 4px; border-radius: var(--vk-radius-sm); background: transparent;
    font: 600 15px/1.1 var(--vk-font-body); font-variant-caps: all-small-caps; letter-spacing: 0.07em;
  }
  :global(:where([data-vibe="paper"])) .sw-pill__mark { width: 7px; height: 7px; border-radius: 1px; transform: translateY(1px); }
  :global(:where([data-vibe="paper"])) .sw-pill[data-tone="neutral"] { --td: var(--vk-color-faint); }

  /* Studio: mono, square marks; failed is ink. */
  :global(:where([data-vibe="studio"])) .sw-pill {
    height: 24px; padding: 0 8px 0 6px; border-radius: 0; background: transparent;
    font: 600 12px/1 var(--vk-font-mono); color: var(--vk-color-muted); box-shadow: inset 0 0 0 1px var(--vk-color-line);
  }
  :global(:where([data-vibe="studio"])) .sw-pill__mark { width: 10px; height: 10px; border-radius: 0; background: transparent; box-shadow: inset 0 0 0 1.5px var(--vk-color-muted); }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="success"] { background: var(--vk-color-accent-wash); color: var(--vk-color-accent-text); box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="success"] .sw-pill__mark { background: var(--vk-stage-6, var(--vk-color-accent)); box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="info"] { color: var(--vk-color-accent-text); box-shadow: inset 0 0 0 1px var(--vk-stage-5, var(--vk-color-accent)); }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="info"] .sw-pill__mark {
    background: repeating-linear-gradient(135deg, var(--vk-stage-5, var(--vk-color-accent)) 0 1.5px, transparent 1.5px 3.5px);
    box-shadow: inset 0 0 0 1.5px var(--vk-stage-5, var(--vk-color-accent));
  }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="warning"] { color: var(--vk-color-text); box-shadow: inset 0 0 0 1px var(--vk-color-rule); }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="warning"] .sw-pill__mark {
    box-shadow: inset 0 0 0 1.5px var(--vk-color-text);
    background: linear-gradient(var(--vk-color-text), var(--vk-color-text)) 50% 100% / 100% 50% no-repeat;
  }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="danger"] { background: var(--vk-color-text); color: var(--vk-color-bg); box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="danger"] .sw-pill__mark { display: inline-grid; place-items: center; background: none; box-shadow: none; font: 700 11px/1 var(--vk-font-body); font-style: normal; }
  :global(:where([data-vibe="studio"])) .sw-pill[data-tone="danger"] .sw-pill__mark::before { content: "×"; }

  /* Quiet: a bordered word; the border style is the tone. */
  :global(:where([data-vibe="quiet"])) .sw-pill {
    display: inline-block; padding: 0 8px; border-radius: var(--vk-radius-sm); line-height: 1.6;
    font: 700 1.125rem/1.6 var(--vk-font-body); background: transparent; color: var(--vk-color-text);
    box-shadow: none; border: 2px solid var(--vk-color-line);
  }
  :global(:where([data-vibe="quiet"])) .sw-pill__mark { display: none; }
  :global(:where([data-vibe="quiet"])) .sw-pill[data-tone="success"] { border-color: var(--vk-color-accent); color: var(--vk-color-accent-text); background: var(--vk-color-accent-wash); }
  :global(:where([data-vibe="quiet"])) .sw-pill[data-tone="danger"] { border: 4px double var(--vk-color-text); line-height: 1.35; }
  :global(:where([data-vibe="quiet"])) .sw-pill[data-tone="warning"] { border-style: dashed; border-color: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-pill[data-tone="info"] { border-color: var(--vk-color-faint); }
  @media (max-width: 1099px) {
    :global(:where([data-vibe="daylight"])) .sw-pill { padding: 3px 10px; font-size: 0.8125rem; }
    :global(:where([data-vibe="quiet"])) .sw-pill { font-size: 1rem; }
  }
</style>
