/**
 * Copyright (c) 2023-2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// Not a pixel snapshot: the events view shows live timestamps and a count that change every run, so
// this checks the row content instead. The e2e suite denies a duplicate Ingress, so the view shows
// one gatekeeper-webhook event. The row is a server-rendered <details class="event-row">; the
// Action ("deny") lives in the collapsed detail, which is still in the DOM that toContainText reads.
test("events view shows a gatekeeper admission event", async ({ page }) => {
  await page.goto("events/");
  const row = page.locator(".event-row", { hasText: "K8sUniqueIngressHost" });
  await expect(row).toContainText("FailedAdmission");
  await expect(row).toContainText("deny");
  await expect(row).toContainText("unique-ingress-host");
});
