// Copies vibekit's first-frame appearance script into static/, so it is served as a file:
// the CSP allows no inline script but SvelteKit's own bootstrap (see internal/web/web.go).
import { copyFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
mkdirSync(join(web, "static"), { recursive: true });
copyFileSync(join(web, "node_modules/@superjackfruit/vibekit/dist/appearance-head.js"), join(web, "static/appearance-head.js"));
