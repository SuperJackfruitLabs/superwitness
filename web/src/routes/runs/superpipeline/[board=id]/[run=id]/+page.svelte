<script lang="ts">
  import { page } from "$app/state";
  import RunPage from "$lib/components/RunPage.svelte";
</script>

<RunPage board={page.params.board ?? ""} run={page.params.run ?? ""} query={page.url.search.replace(/^\?/, "")} />
