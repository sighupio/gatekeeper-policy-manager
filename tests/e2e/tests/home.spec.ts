/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// The Home dashboard rolls up Gatekeeper's audit across the cluster. The e2e pipeline waits for the
// audit to run before this stage (the "Wait for Gatekeeper's audit to run" step in tests/tests.sh),
// so the violation counts are already converged here and no per-test retry is needed.
//
// The lede carries a ticking "updated Ns ago" hint whose width changes as it counts, so masking the
// hint span alone leaves a moving edge; mask the whole lede <p> instead -- a block, so its box is
// full-width and stable regardless of the hint text.
test("page home snapshot", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveScreenshot({
    maxDiffPixels: 100,
    fullPage: true,
    mask: [page.locator(".dash-head p")],
  });
});
