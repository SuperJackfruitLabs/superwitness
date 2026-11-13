#!/bin/sh
# The release's production smoke (the .github/release/smoke.sh hook; the workflow retries
# it): the staging smoke against the public URL, signing in at production Accounts.
set -eu
EXPECT_ISSUER=https://accounts.superjackfruit.com exec sh deploy/staging/smoke.sh https://app.superwitness.dev
