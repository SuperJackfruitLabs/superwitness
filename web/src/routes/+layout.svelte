<script lang="ts">
  import "../app.css";
  import { onDestroy } from "svelte";
  import { page } from "$app/state";
  import { VibeRoot } from "@superjackfruit/vibekit";
  import { ApiError } from "$lib/api";
  import { appearance } from "$lib/appearance.svelte";
  import { fetched } from "$lib/data/fetched.svelte";
  import { session } from "$lib/session.svelte";
  import EdgePage from "$lib/components/EdgePage.svelte";
  import Shell from "$lib/components/Shell.svelte";
  import Loading from "$lib/ui/Loading.svelte";
  import type { Me } from "$lib/types";
  import { setMe } from "$lib/me";

  let { children } = $props();

  // Before the first render, so VibeRoot starts from the cookie, not the defaults (no flash).
  appearance.init();
  onDestroy(() => appearance.dispose());

  // /v1/me first: who is signed in decides everything else.
  const me = fetched<Me>(() => "/v1/me");
  setMe(() => me.data ?? null);
  const path = $derived(page.url.pathname);
  const query = $derived(page.url.search.replace(/^\?/, ""));
  // A 401 from any later call ends the session; Sign in comes back to this exact path and query.
  const expired = new ApiError(401, "unauthenticated", "");
</script>

<VibeRoot vibe={appearance.value.vibe} theme={appearance.theme} accent={appearance.accent} product="superwitness">
  {#if me.error}
    <EdgePage error={me.error} next={path + (query ? `?${query}` : "")} retry={me.reload} />
  {:else if session.expired}
    <EdgePage error={expired} next={path + (query ? `?${query}` : "")} />
  {:else if !me.data}
    <Loading bare />
  {:else}
    <Shell me={me.data} {path} {query}>
      {@render children()}
    </Shell>
  {/if}
</VibeRoot>
