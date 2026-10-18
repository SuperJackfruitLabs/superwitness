# UI building blocks: vibekit or bespoke

Each need, what draws it, and why.

| Need | Uses | Reason |
|---|---|---|
| Edge-page actions (Sign in, Sign out, Try again, Continue, All runs, Rubrics) | vibekit `Button` (primary / secondary / text, `href` for links) | Matches the mockups' `vk-button` in every vibe; keeps 44px targets. |
| Focus layout of the edge pages (one centred column, name above) | bespoke `Panel.svelte` | vibekit has no focus/edge layout; the four vibes draw it differently (SW-SignedOut, SW-Edge). |
| superwitness mark | bespoke `Mark.svelte` | vibekit `Wordmark` is the Super Jackfruit Labs bulb, not the product's eye mark the mockups draw. |
| Refusal line (rate limited, origin mismatch, server error) | bespoke `ErrorPanel.svelte` | vibekit `Notice` draws a titled box; the mockups draw one line (wash in Daylight, "✕" in Paper, ink bar in Studio and Quiet) and it must keep the old exact text. |
| In-shell edge states (run, rubric, page not found) | bespoke `InShell.svelte` | A heading, a line and a link; vibekit `EmptyState` adds its own title level and words. |
| Loading | bespoke `Loading.svelte` | vibekit `Loading` hides its label for screen readers only in Daylight and Paper; the mockups keep the words "Loading…" visible in every vibe with per-vibe placeholders. |
| Empty line | bespoke `Empty.svelte` | One muted line; vibekit `EmptyState` is a page-size block with a title. |
| Warning, info and danger-text tones | `src/lib/tones.css` | vibekit has no warning/info tones or danger text-on-wash; hexes from the artboards. |
