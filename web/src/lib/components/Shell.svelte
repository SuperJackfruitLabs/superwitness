<script lang="ts" module>
  export const VIEWS = [
    { label: "All", to: "/", icon: "list" },
    { label: "Needs verdict", to: "/?needs_verdict=true", icon: "alert" },
    { label: "Failed", to: "/?status=failed", icon: "x" },
    { label: "Waiting", to: "/?status=waiting", icon: "clock" },
  ] as const;
</script>

<script lang="ts">
  import type { Snippet } from "svelte";
  import { fetched } from "../data/fetched.svelte";
  import { signOut } from "../signout.svelte";
  import type { Me, ScopeEntry } from "../types";
  import Mark from "../ui/Mark.svelte";
  import AccountMenu from "./AccountMenu.svelte";
  import SignedOutLingering from "./SignedOutLingering.svelte";
  import SourcesDot from "./SourcesDot.svelte";

  // Shell wraps every signed-in page: the wordmark (the page's one home link), the Menu button on a
  // phone, the sections, the sources dot and the account control. path and query are the current
  // URL's (the layout passes page.url); they decide aria-current. assign is for tests.
  let {
    me,
    path,
    query,
    assign,
    children,
  }: { me: Me; path: string; query: string; assign?: (to: string) => void; children: Snippet } = $props();

  const here = $derived(query ? `${path}?${query}` : path);
  const out = signOut((to) => (assign ? assign(to) : window.location.assign(to)));
  const scopes = fetched<{ scopes: ScopeEntry[] }>(() => "/v1/scopes");
  const boards = $derived(scopes.data?.scopes ?? []);

  let open = $state(false);
  let account: AccountMenu | undefined = $state();
  // The menu closes whenever the URL changes.
  $effect(() => {
    void here;
    open = false;
    account?.close();
  });
</script>

{#snippet icon(name: string)}
  <svg class="sw-ico" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true" focusable="false">
    {#if name === "list"}<path d="M4 6h16M4 12h16M4 18h16" />
    {:else if name === "alert"}<path d="M12 7v6M12 16v1" /><circle cx="12" cy="12" r="9" />
    {:else if name === "x"}<path d="M6 6l12 12M18 6L6 18" />
    {:else if name === "clock"}<circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" />
    {:else if name === "board"}<rect x="3" y="4" width="18" height="16" /><path d="M9 4v16M15 4v16" />
    {:else}<path d="M5 4h14v16H5zM9 9h6M9 13h6" />{/if}
  </svg>
{/snippet}

{#if out.state.kind === "lingering"}
  <SignedOutLingering />
{:else}
  <div class="sw-shell" class:open>
    <div class="sw-side">
      <header class="sw-top">
        <a class="sw-wm" href="/"><Mark size={22} /><span>superwitness</span></a>
        <button class="sw-menu" type="button" aria-expanded={open} aria-controls="nav" onclick={() => (open = !open)}>
          <span class="sw-menu__x" aria-hidden="true">{open ? "×" : ""}</span>Menu
        </button>
      </header>
      <nav id="nav" aria-label="Main">
        <div class="sw-grp">
          <h2>Runs</h2>
          <ul>
            {#each VIEWS as v (v.to)}
              <li><a href={v.to} aria-current={here === v.to ? "page" : undefined}>{@render icon(v.icon)}<span>{v.label}</span></a></li>
            {/each}
          </ul>
        </div>
        {#if boards.length > 0}
          <div class="sw-grp">
            <h2>Boards</h2>
            <ul>
              {#each boards as s (`${s.source}/${s.id}`)}
                {@const to = `/?scope=${encodeURIComponent(s.id)}`}
                <li><a href={to} aria-current={here === to ? "page" : undefined}>{@render icon("board")}<span>{s.name ?? s.id}</span></a></li>
              {/each}
            </ul>
          </div>
        {/if}
        <div class="sw-grp">
          <h2>Library</h2>
          <ul>
            <li><a href="/rubrics" aria-current={path.startsWith("/rubrics") ? "page" : undefined}>{@render icon("rubric")}<span>Rubrics</span></a></li>
          </ul>
        </div>
        <div class="sw-foot"><SourcesDot /></div>
      </nav>
      <div class="sw-account">
        <AccountMenu bind:this={account} {me} out={out.state} signOut={out.run} />
      </div>
    </div>
    <main class="sw-content">{@render children()}</main>
  </div>
{/if}

<style>
  /* Daylight (the base): a raised left column; the account chip top right of the content. */
  .sw-shell { position: relative; box-sizing: border-box; display: grid; grid-template-columns: 240px minmax(0, 1fr); column-gap: var(--vk-space-5, 24px); max-width: 1440px; margin: 0 auto; padding: 16px 24px 48px; }
  .sw-side { display: contents; }
  .sw-top { grid-column: 1; grid-row: 1; position: sticky; top: 16px; z-index: 2; display: flex; align-items: center; padding: 16px 12px 0; border-radius: var(--vk-radius-lg) var(--vk-radius-lg) 0 0; background: var(--vk-color-surface); }
  .sw-wm { display: inline-flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 6px; color: var(--vk-color-text); text-decoration: none; font: 800 1.25rem/1 var(--vk-font-display); letter-spacing: -0.01em; }
  .sw-wm :global(.sw-mark) { width: 40px; height: 40px; }
  .sw-menu { display: none; }
  nav { grid-column: 1; grid-row: 2 / span 2; position: sticky; top: 76px; align-self: start; box-sizing: border-box; display: flex; flex-direction: column; gap: var(--vk-space-5, 24px); min-height: calc(100vh - 92px); padding: 20px 12px 16px; border-radius: 0 0 var(--vk-radius-lg) var(--vk-radius-lg); background: var(--vk-color-surface); box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); }
  .sw-grp h2 { margin: 0; padding: 0 12px 6px; font: 700 0.75rem/1 var(--vk-font-body); letter-spacing: 0.08em; text-transform: uppercase; color: var(--vk-color-muted); }
  .sw-grp ul { margin: 0; padding: 0; list-style: none; }
  .sw-grp a { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 12px; border-radius: var(--vk-radius-sm); color: var(--vk-color-text); text-decoration: none; font-weight: 600; overflow-wrap: anywhere; }
  .sw-grp a:hover { background: var(--vk-color-raised); }
  .sw-grp a[aria-current="page"] { background: var(--vk-color-accent-wash); color: var(--vk-color-accent-text); font-weight: 800; box-shadow: inset 3px 0 0 var(--vk-color-accent); }
  .sw-ico { display: none; flex: none; }
  .sw-foot { margin-top: auto; }
  .sw-account { grid-column: 2; grid-row: 1; justify-self: end; margin-bottom: 12px; }
  .sw-content { grid-column: 2; grid-row: 2; min-width: 0; }

  /* Paper: a ledger column, ruled; the account in its foot. */
  :global(:where([data-vibe="paper"])) .sw-shell { max-width: none; min-height: 100vh; padding: 0; column-gap: 0; grid-template-rows: auto 1fr auto; }
  :global(:where([data-vibe="paper"])) .sw-top { position: static; margin: 16px 24px 0 32px; padding: 6px 0; min-height: 56px; border-radius: 0; background: none; border-top: 1px solid var(--vk-color-text); border-bottom: 3px double var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-wm { padding: 0; font: 600 24px/1 var(--vk-font-display); font-variant-caps: all-small-caps; letter-spacing: 0.06em; }
  :global(:where([data-vibe="paper"])) .sw-wm :global(.sw-mark) { width: 28px; height: 28px; }
  :global(:where([data-vibe="paper"])) nav { grid-row: 2; align-self: stretch; position: static; min-height: 0; gap: 24px; padding: 24px 24px 0 32px; border-radius: 0; background: none; box-shadow: none; }
  :global(:where([data-vibe="paper"])) .sw-top, :global(:where([data-vibe="paper"])) nav, :global(:where([data-vibe="paper"])) .sw-account { border-right: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-top { margin: 0; padding: 22px 24px 6px 32px; border-top: 0; border-bottom: 0; background: linear-gradient(var(--vk-color-text), var(--vk-color-text)) 32px 16px / calc(100% - 56px) 1px no-repeat, linear-gradient(var(--vk-color-text), var(--vk-color-text)) 32px calc(100% - 3px) / calc(100% - 56px) 1px no-repeat, linear-gradient(var(--vk-color-text), var(--vk-color-text)) 32px 100% / calc(100% - 56px) 1px no-repeat; padding-bottom: 12px; }
  :global(:where([data-vibe="paper"])) .sw-grp h2 { padding: 0 0 8px; border-bottom: 1px solid var(--vk-color-text); font: 600 15px/1 var(--vk-font-body); font-variant-caps: all-small-caps; text-transform: none; letter-spacing: 0.12em; }
  :global(:where([data-vibe="paper"])) .sw-grp a { padding: 0 0 0 12px; border-radius: 0; border-bottom: 1px solid var(--vk-color-line); font: 500 18px/1.2 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-grp a:hover { background: none; text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="paper"])) .sw-grp a[aria-current="page"] { background: none; color: var(--vk-color-text); font-weight: 500; font-style: italic; text-decoration: underline; text-decoration-thickness: 1px; text-underline-offset: 5px; }
  :global(:where([data-vibe="paper"])) .sw-foot { margin-top: auto; padding-top: 12px; border-top: 1px solid var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-account { grid-column: 1; grid-row: 3; justify-self: stretch; margin: 0; padding: 0 24px 20px 32px; }
  :global(:where([data-vibe="paper"])) .sw-content { grid-row: 1 / -1; padding: 16px 56px 56px; }

  /* Studio: the black ink rail with labelled icons; sources and account at its foot. */
  :global(:where([data-vibe="studio"])) .sw-shell { max-width: none; min-height: 100vh; padding: 0; column-gap: 0; grid-template-rows: auto 1fr auto; }
  :global(:where([data-vibe="studio"])) .sw-top, :global(:where([data-vibe="studio"])) nav, :global(:where([data-vibe="studio"])) .sw-account { background: var(--vk-color-text); color: var(--vk-color-bg); }
  :global(:where([data-vibe="studio"])) .sw-top { position: static; padding: 16px 12px 18px; border-radius: 0; }
  :global(:where([data-vibe="studio"])) .sw-wm { color: var(--vk-color-bg); font: 800 19px/1 var(--vk-font-body); font-stretch: 85%; }
  :global(:where([data-vibe="studio"])) .sw-wm :global(.sw-mark) { width: 32px; height: 32px; }
  :global(:where([data-vibe="studio"])) nav { grid-row: 2; align-self: stretch; position: static; min-height: 0; gap: 0; padding: 0 12px; border-radius: 0; box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-grp h2 { margin: 14px 6px 4px; padding: 0; color: inherit; font: 700 11px/1 var(--vk-font-body); letter-spacing: 0.12em; opacity: 0.7; }
  :global(:where([data-vibe="studio"])) .sw-grp a { gap: 12px; min-height: 40px; margin-top: 2px; padding: 0 10px; border-radius: 0; border-left: 3px solid transparent; color: var(--vk-color-bg); font: 600 14px var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-grp a:hover { background: none; text-decoration: underline; text-underline-offset: 3px; }
  :global(:where([data-vibe="studio"])) .sw-grp a[aria-current="page"] { background: var(--vk-color-bg); color: var(--vk-color-text); border-left-color: var(--vk-color-accent); box-shadow: none; font-weight: 800; }
  :global(:where([data-vibe="studio"])) .sw-ico { display: block; }
  :global(:where([data-vibe="studio"])) .sw-foot { margin-top: auto; padding-top: 14px; border-top: 1px solid var(--vk-color-bg); }
  :global(:where([data-vibe="studio"])) .sw-account { grid-column: 1; grid-row: 3; justify-self: stretch; margin: 0; padding: 6px 12px 16px; }
  :global(:where([data-vibe="studio"])) .sw-content { grid-row: 1 / -1; padding: 0 40px 40px; }

  /* Quiet: the name, then a plain list of links above the content. */
  :global(:where([data-vibe="quiet"])) .sw-shell { display: block; max-width: 1040px; padding: 0 40px; font-size: 1.125rem; line-height: 1.6; }
  :global(:where([data-vibe="quiet"])) .sw-side { display: block; position: relative; }
  :global(:where([data-vibe="quiet"])) .sw-top { position: static; padding: 16px 0 12px; border-radius: 0; background: none; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="quiet"])) .sw-wm { padding: 0; font: 700 1.125rem/1 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-wm :global(.sw-mark) { width: 14px; height: 14px; }
  :global(:where([data-vibe="quiet"])) nav { position: static; min-height: 0; flex-direction: row; flex-wrap: wrap; align-items: center; gap: 8px 40px; padding: 12px 0; border-radius: 0; background: none; box-shadow: none; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="quiet"])) .sw-grp { display: flex; flex-wrap: wrap; align-items: center; gap: 0 16px; }
  :global(:where([data-vibe="quiet"])) .sw-grp h2 { padding: 0; margin-right: 4px; font: 400 1rem/1 var(--vk-font-body); letter-spacing: 0; text-transform: none; }
  :global(:where([data-vibe="quiet"])) .sw-grp h2::after { content: ":"; }
  :global(:where([data-vibe="quiet"])) .sw-grp ul { display: flex; flex-wrap: wrap; gap: 0 16px; }
  :global(:where([data-vibe="quiet"])) .sw-grp a { padding: 0; border-radius: 0; font-weight: 400; text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-grp a:hover { background: none; }
  :global(:where([data-vibe="quiet"])) .sw-grp a[aria-current="page"] { background: none; color: var(--vk-color-text); font-weight: 700; text-decoration: none; box-shadow: inset 0 -3px 0 var(--vk-color-accent); }
  :global(:where([data-vibe="quiet"])) .sw-foot { position: absolute; top: 16px; left: 200px; margin: 0; }
  :global(:where([data-vibe="quiet"])) .sw-account { position: absolute; top: 16px; right: 0; margin: 0; }

  /* Phone: a topbar with Menu; the sections open as a block above the content, never over it. */
  @media (max-width: 759px) {
    .sw-shell, :global(:where([data-vibe])) .sw-shell { display: block; max-width: none; padding: 0; }
    .sw-side, :global(:where([data-vibe])) .sw-side { display: block; position: static; }
    .sw-top, :global(:where([data-vibe])) .sw-top { position: sticky; top: 0; z-index: 10; display: flex; justify-content: space-between; gap: 12px; margin: 0; padding: 8px 16px; min-height: 60px; box-sizing: border-box; border-radius: 0; background: var(--vk-color-bg); border-bottom: 2px solid var(--vk-color-line); }
    :global(:where([data-vibe="studio"])) .sw-top { background: var(--vk-color-text); border-bottom: 0; }
    :global(:where([data-vibe="paper"])) .sw-top { border-bottom: 1px solid var(--vk-color-text); }
    .sw-menu { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 16px; border: 0; border-radius: var(--vk-radius-pill); background: var(--vk-color-surface); color: var(--vk-color-accent-text); font: inherit; font-weight: 700; cursor: pointer; box-shadow: inset 0 0 0 2px var(--vk-color-accent); }
    .sw-menu__x:empty { display: none; }
    :global(:where([data-vibe="paper"])) .sw-menu { border-radius: var(--vk-radius-sm); background: var(--vk-color-text); color: var(--vk-color-bg); box-shadow: none; }
    :global(:where([data-vibe="studio"])) .sw-menu { border-radius: 0; background: var(--vk-color-bg); color: var(--vk-color-text); box-shadow: none; font-weight: 800; }
    :global(:where([data-vibe="quiet"])) .sw-menu { border-radius: var(--vk-radius-sm); background: var(--vk-color-bg); color: var(--vk-color-text); box-shadow: inset 0 0 0 2px var(--vk-color-text); font-weight: 400; }
    nav, .sw-account, :global(:where([data-vibe])) nav, :global(:where([data-vibe])) .sw-account { display: none; }
    .open nav, :global(:where([data-vibe])) .open nav { position: static; display: flex; flex-direction: column; align-items: stretch; gap: 16px; min-height: 0; margin: 0; padding: 16px 16px 0; border: 0; border-radius: 0; box-shadow: none; }
    :global(:where([data-vibe="quiet"])) .open .sw-grp { display: block; }
    :global(:where([data-vibe="quiet"])) .open .sw-grp ul { display: block; }
    :global(:where([data-vibe="quiet"])) .open .sw-grp h2::after { content: none; }
    :global(:where([data-vibe="quiet"])) .open .sw-grp a { display: flex; min-height: 44px; }
    .open .sw-foot, :global(:where([data-vibe])) .open .sw-foot { position: static; margin: 0; }
    .open .sw-account, :global(:where([data-vibe])) .open .sw-account { position: static; display: block; justify-self: stretch; margin: 0; padding: 16px 16px 20px; border: 0; border-bottom: 2px solid var(--vk-color-line); }
    .sw-content, :global(:where([data-vibe])) .sw-content { padding: 16px 16px 48px; }
    :global(:where([data-vibe="studio"])) .open nav, :global(:where([data-vibe="studio"])) .open .sw-account { background: var(--vk-color-text); color: var(--vk-color-bg); }
  }
</style>
