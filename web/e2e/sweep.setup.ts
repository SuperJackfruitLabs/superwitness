import { test as setup } from "@playwright/test";
import { AUTH, signIn } from "./guard";

// One session for the whole sweep: sign in once and keep the cookie.
setup("sign in for the sweep", async ({ page, request }) => {
  await signIn(page, request);
  await page.context().storageState({ path: AUTH });
});
