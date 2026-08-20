/**
 * Copyright (c) 2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// The Resources view is the same audit data as Constraints, pivoted onto the objects that break
// policies. Like the constraints page it is server-rendered from an audit that has already settled
// by this stage of the pipeline, so no reload loop is needed here either.
test("page resources snapshot", async ({ page }) => {
  await page.goto("resources/");
  await expect(page).toHaveScreenshot({
    maxDiffPixels: 100,
    fullPage: true,
    mask: [page.locator(".dynamic")],
  });
});

// Not a snapshot: the filter hides rows in the DOM, and the counts come from the audit, so both are
// content rather than pixels. The e2e cluster puts every violation in one namespace, which is also
// the case that must render *without* a sidebar.
test("the view pivots the audit onto resources, and the filter narrows it", async ({
  page,
}) => {
  await page.goto("resources/");

  const rows = page.locator(".nscard .event-row");
  await expect(rows).not.toHaveCount(0);

  // A row is one object, and its detail is one line per policy it breaks — summed across the three
  // modes, not just the blocking one. An absent mode renders as an empty cell.
  const first = rows.first();
  const counted = (await first.locator("summary .cnum").allTextContents())
    .map((c) => Number(c.trim()) || 0)
    .reduce((a, b) => a + b, 0);
  expect(counted).toBeGreaterThan(0);
  await first.click();
  await expect(first.locator(".vline")).toHaveCount(counted);

  // The fixture deliberately runs one constraint per mode, so the pills are all exercised.
  for (const mode of ["deny", "dryrun", "warn"]) {
    await expect(page.locator(`.vline .tag-${mode}`).first()).toBeVisible();
  }

  // Every violation line links back to the constraint card that reported it.
  const href = await first
    .locator(".vline .vl-name a")
    .first()
    .getAttribute("href");
  expect(href).toMatch(/\/constraints#.+--.+/);

  // One namespace, so the sidebar is suppressed rather than rendered with a single entry.
  await expect(page.locator("aside.sidebar")).toHaveCount(0);

  // The filter hides non-matching rows and takes their namespace card with them.
  const name = (await first.locator(".rname").textContent())?.trim() ?? "";
  await page.fill(".vfilter", "definitely-not-a-resource-name");
  await expect(rows.first()).toBeHidden();
  await expect(page.locator(".nscard").first()).toBeHidden();

  await page.fill(".vfilter", name);
  await expect(rows.first()).toBeVisible();
  await expect(page.locator(".nscard").first()).toBeVisible();
});
