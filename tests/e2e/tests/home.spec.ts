/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// The UI is server-rendered, so the whole page is in the initial HTML: there is nothing to wait for
// after the load. toHaveScreenshot still auto-waits for the page to stop changing before comparing.
// Volatile content (timestamps, pod ids) carries class="dynamic" and is masked out of the pixels.
test("page home snapshot", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveScreenshot({
    maxDiffPixels: 100,
    fullPage: true,
    mask: [page.locator(".dynamic")],
  });
});
