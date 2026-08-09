/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// This is not a pixel snapshot. The events view shows live timestamps and a count that change every
// run. A stable screenshot of them needs fixed column widths, which is a UX change made only for
// the test. This test checks the row content instead.
//
// The e2e suite denies a duplicate Ingress, so the view shows one gatekeeper-webhook event. This
// test checks that the frontend renders it. The bats suite checks that the API surfaces it.
test("events view shows a gatekeeper admission event", async ({ page }) => {
  await page.goto("events/");
  const row = page.locator(".euiTableRow", { hasText: "K8sUniqueIngressHost" });
  // waitFor uses the test timeout, not the shorter default of the expects. A slow first fetch of
  // the events then does not make the assertion flaky.
  await row.waitFor();
  await expect(row).toContainText("FailedAdmission");
  await expect(row).toContainText("deny");
  await expect(row).toContainText("unique-ingress-host");
});
