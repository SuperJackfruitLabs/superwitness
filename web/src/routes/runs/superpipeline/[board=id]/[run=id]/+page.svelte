<script lang="ts">
  import { page } from "$app/state";
  import RunPage from "$lib/components/RunPage.svelte";
  import { getMe } from "$lib/me";
  const me = getMe();
</script>

<RunPage board={page.params.board ?? ""} run={page.params.run ?? ""} query={page.url.search.replace(/^\?/, "")} me={me()} />
