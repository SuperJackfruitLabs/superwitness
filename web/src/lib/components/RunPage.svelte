<script lang="ts">
  import RunView from "./RunView.svelte";
  // RunPage keys the run view by its run: another run is a fresh view with no evidence ticked and
  // no drawer open, and a late answer for the old run has nowhere to land.
  let { board, run, query }: { board: string; run: string; query: string } = $props();
</script>

{#key `${board}/${run}`}
  <RunView {board} {run} {query} />
{/key}
