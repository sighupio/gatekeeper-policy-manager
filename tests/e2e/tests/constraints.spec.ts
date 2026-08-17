/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// The page is server-rendered from Gatekeeper's audit status: the violation counts and the per-pod
// status block, both of which fill in over a few audit cycles after deploy. The e2e pipeline waits
// for that to settle before this stage (the "Wait for Gatekeeper's audit to run" step in
// tests/tests.sh checks every constraint is audited and its byPod block is complete), so the page is
// already converged here and needs no reload loop. Volatile content (timestamps, pod ids) carries
// class="dynamic" and is masked out of the pixels.
test("page constraints snapshot", async ({ page }) => {
  await page.goto("constraints/");
  await expect(page).toHaveScreenshot({
    maxDiffPixels: 100,
    fullPage: true,
    mask: [page.locator(".dynamic")],
  });
});
