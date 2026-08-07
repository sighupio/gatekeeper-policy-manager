/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

test("page events snapshot", async ({ page }) => {
  await page.goto("events/");
  // The e2e suite triggers one denied Ingress, so the view shows exactly one event row. Wait for
  // its constraint kind rather than the old empty-state prompt.
  await page.getByText("K8sUniqueIngressHost").waitFor();
  await expect(page).toHaveScreenshot({
    maxDiffPixels: 100,
    fullPage: true,
    mask: [
      page.locator(".dynamic"),
      // First Timestamp, Last Timestamp and Count change every run.
      page.locator(".euiTableRowCell:nth-child(1)"),
      page.locator(".euiTableRowCell:nth-child(2)"),
      page.locator(".euiTableRowCell:nth-child(3)"),
    ],
  });
});
