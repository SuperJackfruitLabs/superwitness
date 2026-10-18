<script lang="ts">
  import { page } from "$app/state";
  import RunsPage from "$lib/components/RunsPage.svelte";
</script>

<RunsPage query={page.url.search.replace(/^\?/, "")} />
