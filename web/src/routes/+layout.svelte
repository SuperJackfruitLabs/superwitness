<script lang="ts">
  import "../app.css";
  import { page } from "$app/state";
  import { fetched } from "$lib/data/fetched.svelte";
  import EdgePage from "$lib/components/EdgePage.svelte";
  import Loading from "$lib/ui/Loading.svelte";

  let { children } = $props();

  // /v1/me first: who is signed in decides everything else. Part 5 adds the shell around children.
  const me = fetched<unknown>(() => "/v1/me");
  const next = $derived(page.url.pathname + page.url.search);
</script>

{#if me.error}
  <EdgePage error={me.error} {next} retry={me.reload} />
{:else if !me.data}
  <Loading bare />
{:else}
  {@render children()}
{/if}
