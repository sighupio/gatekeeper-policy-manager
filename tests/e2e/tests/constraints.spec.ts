/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from '@playwright/test';

test('page constraints snapshot', async ({ page }) => {
  await page.goto('constraints/');
  // Using the span with the page title for some reason stopped working
  // const constraintsSpan = page.locator('span[title="Constraints"]');
  const constraintTitle = page.locator('#enforce-deployment-and-pod-security-controls');
  await constraintTitle.waitFor();
  await expect(page).toHaveScreenshot({ maxDiffPixels: 100, fullPage: true, mask: [page.locator('.dynamic')] });;
});
