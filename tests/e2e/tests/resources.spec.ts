/**
 * Copyright (c) 2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

declare global {
  interface Window {
    __copied: string[];
  }
}

// The Resources view is the same audit data as Constraints, pivoted onto the objects that break
// policies. Like the constraints page it is server-rendered from an audit that has already settled
// by this stage of the pipeline, so no reload loop is needed here either.
test("page resources snapshot", async ({ page }) => {
  await page.goto("resources/");
  await expect(page).toHaveScreenshot({
    maxDiffPixels: 100,
    fullPage: true,
    // The Pod row is the local-path-provisioner Pod, whose name carries a ReplicaSet hash and a
    // generated suffix -- different on every fresh cluster, so its name cell cannot be compared.
    // The fixture's own objects are named, and stay in the pixels.
    mask: [page.locator(".dynamic"), page.locator('[id*="--Pod--"] .rname')],
  });
});

// Not a snapshot: the filter hides rows in the DOM, and the counts come from the audit, so both are
// content rather than pixels.
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

  // Two namespaces in the fixture, so the sidebar is rendered: one entry per namespace, each with a
  // severity bar and a total. The suppressed single-namespace case is covered by the Go tests.
  const entries = page.locator(".nsnav a");
  await expect(entries).toHaveCount(await page.locator(".nscard").count());
  await expect(entries.first().locator(".sevbar")).toBeVisible();
  // Worst first: the namespace with blocking violations leads.
  const denies = await page
    .locator(".nscard .card-head .n-deny")
    .allTextContents();
  expect(Number(denies[0] ?? 0)).toBeGreaterThan(0);

  // The filter hides non-matching rows and takes their namespace card with them.
  const name = (await first.locator(".rname").textContent())?.trim() ?? "";
  await page.fill(".vfilter", "definitely-not-a-resource-name");
  await expect(rows.first()).toBeHidden();
  await expect(page.locator(".nscard").first()).toBeHidden();

  await page.fill(".vfilter", name);
  await expect(rows.first()).toBeVisible();
  await expect(page.locator(".nscard").first()).toBeVisible();
});

// The share link. Not a snapshot: the interesting parts are the clipboard call, the fragment and
// the row state it produces. The clipboard is stubbed rather than read back, because the suite
// reaches GPM over host.docker.internal -- not a secure context, so navigator.clipboard does not
// exist there at all. That is also true of any GPM served over plain HTTP on a hostname, which is
// why copyLink falls back to putting the link in the address bar.
test("a resource row can be linked to, and the link reopens it", async ({
  page,
}) => {
  await page.addInitScript(() => {
    window.__copied = [];
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: (t: string) => {
          window.__copied.push(t);
          return Promise.resolve();
        },
      },
    });
  });
  await page.goto("resources/");

  const row = page.locator(".nscard .event-row").first();
  const id = await row.getAttribute("id");
  // Readable by design, so a link pasted into a channel says what it points at.
  expect(id).toMatch(/^(ns-.+|cluster-scoped)--[A-Za-z]+--.+$/);

  await row.locator(".vlink").click();
  const copied = await page.evaluate(() => window.__copied);
  expect(copied).toEqual([`${page.url().split("#")[0]}#${id}`]);
  // The button reports success in place, and must not have toggled the row it sits in.
  await expect(row.locator(".copy-tick")).toBeVisible();
  await expect(row).not.toHaveAttribute("open", "");

  // Arriving on the link opens that row, even when a filter is hiding it.
  await page.fill(".vfilter", "definitely-not-a-resource-name");
  await expect(row).toBeHidden();
  await page.goto(`resources/#${id}`);
  const linked = page.locator(`[id="${id}"]`);
  await expect(linked).toHaveAttribute("open", "");
  await expect(linked).toBeVisible();
});
