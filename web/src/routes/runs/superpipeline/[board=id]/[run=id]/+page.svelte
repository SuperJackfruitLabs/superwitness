<script lang="ts">
  import { page } from "$app/state";
  import RunView from "$lib/components/RunView.svelte";

  const board = $derived(page.params.board ?? "");
  const run = $derived(page.params.run ?? "");
</script>

<!-- Keyed by the run: another run is a fresh view with no evidence ticked and no drawer open. -->
{#key `${board}/${run}`}
  <RunView {board} {run} query={page.url.search.replace(/^\?/, "")} />
{/key}
