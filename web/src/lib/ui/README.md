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
| The shell (sections, Menu on a phone, account, sources dot) | bespoke `components/Shell.svelte` | vibekit `AppShell`/`AppNav` draw one layout for all vibes; the mockups draw four (Daylight raised column, Paper ruled ledger, Studio ink rail with labelled icons, Quiet a plain list above the content) and the old app's DOM contract (`nav#nav` "Main", "Menu" with `aria-controls="nav"`, inline block on a phone) must hold. |
| Account control (identity, Appearance, Sign out) | bespoke `components/AccountMenu.svelte` | vibekit `Menu` is a pure action list; this one holds the identity block, swaps to the Appearance control in place, and is drawn open inside the phone Menu block. |
| Appearance control | bespoke `components/Appearance.svelte` on vibekit's model (`accentOptions`, `VIBES`, `appearanceCookie` via `appearance.svelte.ts`) | vibekit `AppearancePicker` is a full page with previews, intro and hints (many words the spec does not list); the mockup's "Appearance open" is a compact panel: Vibe, Theme (Follow the sun, Light, Dark), Accent swatches. |
| Sources dot | bespoke `components/SourcesDot.svelte` | No vibekit equivalent; amber uses `tones.css` warn tones. |
| Vibe on `<html>` | vibekit `VibeRoot` (layout) plus `appearance.svelte.ts` | VibeRoot provides the vibe context vibekit components read; the store also writes `data-vibe`/`data-theme` so a choice lands before effects run. |
