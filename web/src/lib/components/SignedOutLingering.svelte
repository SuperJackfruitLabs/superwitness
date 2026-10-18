<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import Panel from "../ui/Panel.svelte";
  // Sign-out answered 503 store_unavailable: signed out in this browser, maybe not on the server.
</script>

<Panel>
  <h1>Signed out</h1>
  <p class="sw-lead">
    You are signed out in this browser. The database did not answer, so the server-side session may outlive this
    sign-out. On a shared computer, sign in again later and sign out once more.
  </p>
  <p class="sw-act"><Button href="/" data-sveltekit-reload>Continue</Button></p>
</Panel>
