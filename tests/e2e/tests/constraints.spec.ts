/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

test("page constraints snapshot", async ({ page }) => {
  // Converging Gatekeeper's audit can take several cycles, so allow well beyond the default 30s.
  test.setTimeout(220_000);

  // The page is server-rendered from Gatekeeper's audit status, which converges a few audit cycles
  // after deploy (and after the [CHART] test churns the cluster right before this runs). Until it
  // settles, a Constraint can show a "not audited yet" banner, and the per-pod status block grows as
  // each controller/audit pod reports in. Both change the page height, so a full-page snapshot taken
  // too early fails on size. The page does not auto-refresh, so reload until it is settled: no
  // banner, and a height that no longer changes between two reloads.
  let previousHeight = -1;
  await expect(async () => {
    await page.goto("constraints/");
    await expect(page.getByText("has probably not audited it yet")).toHaveCount(
      0,
    );
    const height = await page.evaluate(() => document.body.scrollHeight);
    const settled = height === previousHeight;
    previousHeight = height; // advance before asserting, so the next reload compares against it
    expect(
      settled,
      `constraints page height not stable yet (${height}px)`,
    ).toBe(true);
  }).toPass({ timeout: 180_000, intervals: [4_000] });

  await expect(page).toHaveScreenshot({
    maxDiffPixels: 100,
    fullPage: true,
    mask: [page.locator(".dynamic")],
  });
});
