<script lang="ts">
  import type { Me } from "../types";
  import RunView from "./RunView.svelte";
  // RunPage keys the run view by its run: another run is a fresh view with no evidence ticked and
  // no drawer open, and a late answer for the old run has nowhere to land.
  // me is who is signed in (Revise is offered on their own verdicts); null before /v1/me answers.
  let { board, run, query, me = null }: { board: string; run: string; query: string; me?: Me | null } = $props();
</script>

{#key `${board}/${run}`}
  <RunView {board} {run} {query} {me} />
{/key}
