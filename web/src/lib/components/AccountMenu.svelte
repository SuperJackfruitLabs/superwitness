<script lang="ts">
  import type { Me } from "../types";
  import type { SignOutState } from "../signout.svelte";
  import Appearance from "./Appearance.svelte";

  // The account chip and its menu: who is signed in, Appearance, Sign out (session callers only).
  // On a phone the menu is drawn open inside the Menu block (CSS), so everything is one scroll away.
  let { me, out, signOut }: { me: Me; out: SignOutState; signOut: () => void } = $props();

  let open = $state(false);
  let view = $state<"menu" | "appearance">("menu");
  let root: HTMLElement;

  const who = $derived(me.email ?? me.principal);
  // Initials as the mockups draw them: the first letter, then the last digit of the name if there is one (human01: H1).
  const initials = $derived((who.match(/[a-z]/i)?.[0] ?? "?").toUpperCase() + (who.split("@")[0].match(/\d(?=\D*$)/)?.[0] ?? ""));

  export function close() {
    open = false;
    view = "menu";
  }

  $effect(() => {
    if (!open) return;
    const down = (e: PointerEvent) => {
      if (root && !root.contains(e.target as Node)) close();
    };
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    document.addEventListener("pointerdown", down);
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("pointerdown", down);
      document.removeEventListener("keydown", key);
    };
  });
</script>

<div class="sw-acct" bind:this={root}>
  <button class="sw-chip" type="button" aria-haspopup="menu" aria-expanded={open} onclick={() => (open ? close() : (open = true))}>
    <span class="sw-ini" aria-hidden="true">{initials}</span><span class="sw-chip__who" title={me.principal}>{who}</span>
  </button>
  <div class="sw-pop" class:open>
    {#if view === "menu"}
      <!-- Only while open: the chip already shows who, and the old app said it once on the page. -->
      {#if open}<p class="sw-who"><b>{who}</b><span>{me.principal}</span></p>{/if}
      <div role="menu" aria-label={who}>
        <button class="sw-mi" role="menuitem" type="button" onclick={() => (view = "appearance")}>Appearance<span class="sw-chev" aria-hidden="true">›</span></button>
        {#if me.via === "session"}
          <button class="sw-mi" role="menuitem" type="button" onclick={signOut} disabled={out.kind === "busy"}>Sign out</button>
        {/if}
      </div>
    {:else}
      <button class="sw-mi sw-back" type="button" onclick={() => (view = "menu")}><span aria-hidden="true">‹</span>Appearance</button>
      <Appearance />
    {/if}
  </div>
  {#if out.kind === "failed"}
    <p class="sw-failed" role="alert">
      Sign out failed — try again. {out.reason}
      <button class="sw-linkbtn" type="button" onclick={signOut}>Try again</button>
    </p>
  {/if}
</div>

<style>
  .sw-acct { position: relative; display: flex; flex-direction: column; align-items: flex-end; gap: 8px; }
  .sw-chip { display: inline-flex; align-items: center; gap: 8px; max-width: 100%; min-height: 44px; padding: 0 16px 0 4px; border: 0; border-radius: var(--vk-radius-pill); background: var(--vk-color-surface); color: var(--vk-color-text); font: inherit; font-weight: 600; cursor: pointer; box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); }
  .sw-chip__who { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sw-ini { display: inline-grid; place-items: center; flex: none; width: 36px; height: 36px; border-radius: 50%; background: var(--vk-mood-resting); color: var(--vk-face-ink, var(--vk-color-text)); font: 800 12px/1 var(--vk-font-body); }
  .sw-pop { position: absolute; top: calc(100% + 8px); right: 0; z-index: 20; width: 320px; max-width: calc(100vw - 32px); padding: 8px; border-radius: var(--vk-radius-lg); background: var(--vk-color-surface); box-shadow: 0 18px 48px -18px var(--vk-color-ground), inset 0 0 0 2px var(--vk-color-line); text-align: left; }
  .sw-pop:not(.open) { display: none; }
  .sw-who { display: flex; flex-direction: column; gap: 4px; margin: 0 0 6px; padding: 10px 12px 12px; border-bottom: 2px solid var(--vk-color-line); overflow-wrap: anywhere; }
  .sw-who span { font: 400 var(--vk-text-sm)/1.3 var(--vk-font-mono); color: var(--vk-color-muted); }
  .sw-mi { display: flex; align-items: center; justify-content: space-between; gap: 8px; width: 100%; min-height: 44px; padding: 0 12px; border: 0; border-radius: var(--vk-radius-sm); background: transparent; color: var(--vk-color-text); font: inherit; font-weight: 600; text-align: left; cursor: pointer; }
  .sw-mi:hover { background: var(--vk-color-raised); }
  .sw-mi:disabled { opacity: 0.6; cursor: default; }
  .sw-back { justify-content: flex-start; background: var(--vk-color-raised); }
  .sw-failed { margin: 0; max-width: 320px; padding: 10px 12px; border-radius: var(--vk-radius-md); background: var(--sw-danger-wash); color: var(--sw-danger-text); font-size: var(--vk-text-sm); font-weight: 600; text-align: left; }
  .sw-linkbtn { min-height: 44px; padding: 0; border: 0; background: none; color: inherit; font: inherit; font-weight: 700; text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }

  /* Paper and Studio: the chip is the rail's foot (the email, the initial), the menu opens upward. */
  :global(:where([data-vibe="paper"], [data-vibe="studio"])) .sw-acct { align-items: stretch; }
  :global(:where([data-vibe="paper"], [data-vibe="studio"])) .sw-pop { top: auto; bottom: calc(100% + 8px); left: 0; right: auto; width: 260px; }
  :global(:where([data-vibe="paper"])) .sw-chip { min-height: 48px; padding: 0; border-radius: 0; background: none; box-shadow: none; font-weight: 400; font-size: 14px; text-align: left; }
  :global(:where([data-vibe="paper"])) .sw-chip__who { white-space: normal; overflow-wrap: anywhere; line-height: 1.3; }
  :global(:where([data-vibe="paper"])) .sw-ini { width: 32px; height: 32px; background: none; box-shadow: inset 0 0 0 1px var(--vk-color-text); color: var(--vk-color-text); font-weight: 600; }
  :global(:where([data-vibe="paper"])) .sw-pop { padding: 6px; border-radius: var(--vk-radius-sm); box-shadow: inset 0 0 0 1px var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-mi { font-weight: 400; font-size: 15px; }
  :global(:where([data-vibe="paper"])) .sw-failed { background: none; padding: 0; color: var(--sw-danger-text); font-weight: 500; }
  :global(:where([data-vibe="studio"])) .sw-chip { min-height: 48px; padding: 0 6px; border-radius: 0; background: transparent; color: var(--vk-color-bg); box-shadow: inset 0 0 0 1.5px var(--vk-color-bg); text-align: left; }
  :global(:where([data-vibe="studio"])) .sw-chip__who { font: 500 12px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-ini { width: 30px; height: 30px; border-radius: 0; background: var(--vk-color-bg); color: var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-pop { border-radius: 0; background: var(--vk-color-bg); color: var(--vk-color-text); box-shadow: inset 0 0 0 1.5px var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-mi { border-radius: 0; font-weight: 800; }
  :global(:where([data-vibe="studio"])) .sw-failed { border-radius: 0; background: var(--vk-color-bg); color: var(--vk-color-text); }

  /* Quiet: an underlined text button with a ringed initial. */
  :global(:where([data-vibe="quiet"])) .sw-chip { padding: 0; background: none; box-shadow: none; font-weight: 400; }
  :global(:where([data-vibe="quiet"])) .sw-chip__who { text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-ini { width: 32px; height: 32px; background: none; box-shadow: inset 0 0 0 2px var(--vk-color-text); color: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-pop { border-radius: var(--vk-radius-sm); box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-mi { font-weight: 400; text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-failed { background: none; border-radius: 0; padding: 4px 0 4px 16px; box-shadow: inset 4px 0 0 var(--vk-color-text); color: var(--vk-color-text); font-weight: 400; }

  /* Phone: the menu sits open inside the Menu block. */
  @media (max-width: 759px) {
    .sw-acct, :global(:where([data-vibe])) .sw-acct { align-items: stretch; }
    .sw-chip, :global(:where([data-vibe])) .sw-chip { pointer-events: none; box-shadow: none; background: none; padding: 0; color: inherit; }
    .sw-pop, .sw-pop:not(.open), :global(:where([data-vibe])) .sw-pop { position: static; display: block; width: auto; max-width: none; padding: 0; background: none; color: inherit; box-shadow: none; }
    .sw-who { display: none; }
    .sw-failed { max-width: none; }
    :global(:where([data-vibe="studio"])) .sw-mi { margin-top: 8px; color: inherit; box-shadow: inset 0 0 0 1.5px currentColor; }
    :global(:where([data-vibe="studio"])) .sw-mi:hover { background: none; text-decoration: underline; }
  }
</style>
