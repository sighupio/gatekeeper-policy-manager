/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// The UI is server-rendered, so the whole page is in the initial HTML: there is nothing to wait for
// after the load. toHaveScreenshot still auto-waits for the page to stop changing before comparing.
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
