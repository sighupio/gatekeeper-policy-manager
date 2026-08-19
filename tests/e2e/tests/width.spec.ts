/**
 * Copyright (c) 2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// The topbar toggle lifts the 1360px cap the layout inherited from the 1.x UI. Not a pixel snapshot:
// the default is unchanged, so the baselines would not see any of this. A wide window, because the
// whole point is the room a wide window has.
test.use({ viewport: { width: 2560, height: 900 } });

const pageWidth = (p: import("@playwright/test").Page) =>
  p.evaluate(() =>
    Math.round(document.querySelector(".page")!.getBoundingClientRect().width),
  );

test("the layout is capped until asked otherwise, and then it fills the window", async ({
  page,
}) => {
  await page.goto("constrainttemplates/");

  const capped = await pageWidth(page);
  expect(
    capped,
    "the default layout is capped well short of a 2560px window",
  ).toBeLessThan(1500);

  await page.click('button[aria-label="Layout width"]');
  await expect
    .poll(() => pageWidth(page), { timeout: 3000 })
    .toBeGreaterThan(capped + 500);

  // Prose does not get the extra room; the tables and cards do. Otherwise a description runs the
  // whole window and nobody can read it.
  const lede = await page.evaluate(() =>
    Math.round(
      document.querySelector(".view-head p")!.getBoundingClientRect().width,
    ),
  );
  const full = await pageWidth(page);
  expect(
    lede,
    "the lede should stay readable, not span the window",
  ).toBeLessThan(full / 2);

  await page.click('button[aria-label="Layout width"]');
  await expect.poll(() => pageWidth(page), { timeout: 3000 }).toBe(capped);
});

// Every view is a fresh server render, so the choice has to be applied before the page paints or it
// flashes from centred to full on each navigation. The head script is synchronous, which means the
// attribute is already set by the time the DOM is ready.
test("the choice survives navigation, and lands before the page is built", async ({
  page,
}) => {
  await page.goto("constrainttemplates/");
  await page.click('button[aria-label="Layout width"]');
  await expect
    .poll(() => pageWidth(page), { timeout: 3000 })
    .toBeGreaterThan(1500);

  for (const view of ["constraints/", "events/", "mutations/"]) {
    await page.goto(view, { waitUntil: "domcontentloaded" });
    const atDomReady = await page.evaluate(() =>
      document.documentElement.hasAttribute("data-full-width"),
    );
    expect(
      atDomReady,
      `${view} rendered capped before the toggle was applied`,
    ).toBe(true);
  }
});
