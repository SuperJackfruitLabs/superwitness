import adapter from "@sveltejs/adapter-static";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

/** @type {import('@sveltejs/kit').Config} */
export default {
  preprocess: vitePreprocess(),
  kit: {
    // The console is a client-only app served by the Go binary, which answers every
    // extension-less path that is not /v1, /mcp, /auth or /health with index.html.
    adapter: adapter({ pages: "../internal/web/dist", assets: "../internal/web/dist", fallback: "index.html", precompress: false, strict: false }),
    paths: { base: "" },
    router: { type: "pathname" },
    // The committed dist must be byte-identical on every build (make web-check), and the inline
    // bootstrap script's hash must not change: SvelteKit's default version name is a timestamp.
    version: { name: "superwitness" },
  },
};
