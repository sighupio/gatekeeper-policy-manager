/**
 * Copyright (c) 2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect, type Page } from "@playwright/test";

// The sidebar scroll-spy (static/ssr/sidebar-spy.js) marks the entry whose card is in view. Clicking
// an entry near the end of the list used to highlight an earlier one: the page runs out of scroll,
// so the last cards never reach the observer's detection band and an earlier card kept the mark.
// Not a pixel snapshot, because the fault is in a class the screenshots barely show.
//
// Pinned viewport, taller than the 1280x720 default. The fault needs the last card to be unable to
// reach the band, which happens when the window is tall relative to the content below that card's
// top -- so a taller window makes it worse, not better. At 720 the last Constraint Template card is
// long enough to reach the band on its own and the spec would test nothing; 1000 puts both views
// squarely in the broken geometry. assertTheBugIsStillPossible enforces that rather than trusting it.
test.use({ viewport: { width: 1280, height: 1000 } });

const active = (page: Page) =>
  page.evaluate(
    () =>
      document.querySelector(".sidebar-nav a.active")?.getAttribute("href") ??
      null,
  );

// The pin marks the clicked entry at once, while the smooth scroll is still running, so anything
// that measures geometry has to wait for the page to stop moving first.
async function settle(page: Page) {
  let previous = -1;
  for (let i = 0; i < 25; i++) {
    const y = await page.evaluate(() => Math.round(scrollY));
    if (y === previous) return;
    previous = y;
    await page.waitForTimeout(120);
  }
}

// Fails loudly rather than passing vacuously: if the layout ever gives the last card room to reach
// the band, these tests stop covering the bug and someone needs to know.
async function assertTheBugIsStillPossible(page: Page) {
  const state = await page.evaluate(() => {
    const doc = document.documentElement;
    const max = doc.scrollHeight - innerHeight;
    window.scrollTo({ top: max, behavior: "instant" });
    const cards = [...document.querySelectorAll(".card[id]")];
    const last = cards[cards.length - 1];
    return {
      maxScroll: max,
      lastCardTopAtBottom: last
        ? Math.round(last.getBoundingClientRect().top)
        : null,
      bandBottom: Math.round(innerHeight * 0.4),
    };
  });
  expect(
    state.maxScroll,
    "the view must scroll for the scroll-spy to mean anything",
  ).toBeGreaterThan(0);
  expect(
    state.lastCardTopAtBottom,
    "the last card now reaches the detection band, so this spec no longer covers the bug it was written for",
  ).toBeGreaterThan(state.bandBottom);
  await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
  await page.waitForTimeout(200);
}

for (const view of ["constraints", "constrainttemplates"]) {
  test(`${view}: the sidebar marks the entry that was clicked, including the last`, async ({
    page,
  }) => {
    await page.goto(`${view}/`);
    await assertTheBugIsStillPossible(page);

    const entries = await page.$$eval(".sidebar-nav a", (as) =>
      as.map((a) => a.getAttribute("href") ?? ""),
    );
    expect(entries.length).toBeGreaterThan(1);

    for (const href of entries) {
      await page.evaluate(() =>
        window.scrollTo({ top: 0, behavior: "instant" }),
      );
      await page.waitForTimeout(200);
      await page.click(`.sidebar-nav a[href="${href}"]`);
      await expect
        .poll(() => active(page), { message: `clicked ${href}`, timeout: 3000 })
        .toBe(href);
    }
  });

  test(`${view}: scrolling to the end marks the last entry`, async ({
    page,
  }) => {
    await page.goto(`${view}/`);
    await assertTheBugIsStillPossible(page);

    const entries = await page.$$eval(".sidebar-nav a", (as) =>
      as.map((a) => a.getAttribute("href") ?? ""),
    );
    // No fragment in the URL: this is the reader scrolling by hand, not following a link.
    await page.evaluate(() => {
      history.replaceState(null, "", location.pathname);
      window.scrollTo({
        top: document.documentElement.scrollHeight,
        behavior: "instant",
      });
    });
    await expect
      .poll(() => active(page), { timeout: 3000 })
      .toBe(entries[entries.length - 1]);

    await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
    await expect.poll(() => active(page), { timeout: 3000 }).toBe(entries[0]);
  });
}

// Clicking the entry that is already marked must not disturb it. `pointerdown` releases the click
// pin on the way into the click; recomputing there marked whatever sits under the reading line for
// one frame before the click marked the clicked entry, which reads as a flick to the entry below and
// back. It only shows on a tall window, where the line is far down the page, so pin the height and
// assert the geometry rather than hoping for it.
test.describe("on a tall window", () => {
  test.use({ viewport: { width: 1280, height: 1200 } });

  for (const view of ["constraints", "constrainttemplates"]) {
    test(`${view}: clicking the marked entry again does not flick`, async ({
      page,
    }) => {
      await page.goto(`${view}/`);
      const entries = await page.$$eval(".sidebar-nav a", (as) =>
        as.map((a) => a.getAttribute("href") ?? ""),
      );
      const target = entries[1];

      await page.click(`.sidebar-nav a[href="${target}"]`);
      await expect.poll(() => active(page), { timeout: 3000 }).toBe(target);
      await settle(page);

      // The flick needs the reading line to sit past the top of the following section, or the
      // recompute would have answered the clicked entry anyway and this proves nothing.
      const geometry = await page.evaluate((href) => {
        const doc = document.documentElement;
        const line = Math.max(
          80,
          innerHeight - Math.max(0, doc.scrollHeight - innerHeight - scrollY),
        );
        const cards = [...document.querySelectorAll(".card[id]")];
        const i = cards.findIndex((c) => "#" + c.id === href);
        const next = cards[i + 1];
        return {
          line,
          nextTop: next ? next.getBoundingClientRect().top : Infinity,
        };
      }, target);
      expect(
        geometry.line,
        "the reading line no longer reaches past the next section, so this spec cannot see the flick",
      ).toBeGreaterThan(geometry.nextTop);

      // Record every change of the marked entry across the second click.
      await page.evaluate(() => {
        window.__marks = [];
        new MutationObserver(() => {
          const a =
            document
              .querySelector(".sidebar-nav a.active")
              ?.getAttribute("href") ?? null;
          if (window.__marks.at(-1) !== a) window.__marks.push(a);
        }).observe(document.querySelector(".sidebar-nav"), {
          subtree: true,
          attributes: true,
          attributeFilter: ["class"],
        });
      });
      await page.click(`.sidebar-nav a[href="${target}"]`);
      await page.waitForTimeout(600);

      expect(await page.evaluate(() => window.__marks)).toEqual([]);
      expect(await active(page)).toBe(target);
    });
  }
});

// Three defects an audit of this component turned up, each verified against the running app before
// the fix went in. They share the pin: it is right to hold the reader's choice, wrong to hold it
// when the page moved by other means, or to set it when the page did not move at all.
test.describe("the pin only speaks for this page", () => {
  test("a modified click opens a new tab and must not move the mark", async ({
    page,
  }) => {
    await page.goto("constrainttemplates/");
    const entries = await page.$$eval(".sidebar-nav a", (as) =>
      as.map((a) => a.getAttribute("href") ?? ""),
    );
    const before = await active(page);
    expect(before).toBe(entries[0]);

    // Ctrl-click: the browser opens the fragment in a new tab, this page stays put.
    await page.click(`.sidebar-nav a[href="${entries[4]}"]`, {
      modifiers: ["ControlOrMeta"],
    });
    await page.waitForTimeout(400);

    expect(await page.evaluate(() => Math.round(scrollY))).toBe(0);
    expect(await active(page)).toBe(before);
  });

  test("going Back releases the pin and the mark follows the page", async ({
    page,
  }) => {
    await page.goto("constrainttemplates/");
    const entries = await page.$$eval(".sidebar-nav a", (as) =>
      as.map((a) => a.getAttribute("href") ?? ""),
    );

    await page.click(`.sidebar-nav a[href="${entries[1]}"]`);
    await settle(page);
    await page.click(`.sidebar-nav a[href="${entries[3]}"]`);
    await settle(page);
    expect(await active(page)).toBe(entries[3]);

    await page.goBack();
    await settle(page);
    // Back returns to the second section; the mark must come with it, with no scrolling of our own.
    await expect.poll(() => active(page), { timeout: 3000 }).toBe(entries[1]);
  });
});

// A window taller than the whole page cannot scroll at all, so the reader is at the first section
// and nowhere else. The reading line used to open near the bottom of such a window and mark an entry
// several sections ahead before anything had moved.
test.describe("on a window taller than the page", () => {
  test.use({ viewport: { width: 1280, height: 2600 } });

  test("constraints: a page that fits marks its first entry", async ({
    page,
  }) => {
    await page.goto("constraints/");
    const entries = await page.$$eval(".sidebar-nav a", (as) =>
      as.map((a) => a.getAttribute("href") ?? ""),
    );
    const maxScroll = await page.evaluate(
      () => document.documentElement.scrollHeight - innerHeight,
    );
    expect(
      maxScroll,
      "the window must be taller than the page for this spec to mean anything",
    ).toBeLessThanOrEqual(0);
    expect(await active(page)).toBe(entries[0]);
  });
});

// The mark must be readable by assistive tech, not only visible as a colour.
test("the marked entry carries aria-current", async ({ page }) => {
  await page.goto("constraints/");
  await expect(page.locator(".sidebar-nav a[aria-current]")).toHaveCount(1);
  const [marked, current] = await page.evaluate(() => {
    const a = document.querySelector(".sidebar-nav a.active");
    return [
      a?.getAttribute("href") ?? null,
      a?.getAttribute("aria-current") ?? null,
    ];
  });
  expect(current).toBe("true");
  expect(marked).not.toBeNull();
});

// The sidebar is its own scroll box, so on a long list the marked entry can sit outside it and the
// reader sees no mark anywhere. Verified against the running app before the fix: at the foot of the
// page the marked entry was outside the box. A short window is the reliable way to make the sidebar
// overflow with the handful of objects the e2e cluster has; the spec asserts that it really does,
// rather than passing on a sidebar that never needed to scroll.
test.describe("on a short window", () => {
  test.use({ viewport: { width: 1280, height: 300 } });

  test("constrainttemplates: the mark stays inside the sidebar's own scroll box", async ({
    page,
  }) => {
    await page.goto("constrainttemplates/");

    const overflows = await page.evaluate(() => {
      const box = document.querySelector(".sidebar");
      return box.scrollHeight > box.clientHeight;
    });
    expect(
      overflows,
      "the sidebar must overflow for this spec to mean anything",
    ).toBe(true);

    const maxScroll = await page.evaluate(
      () => document.documentElement.scrollHeight - innerHeight,
    );
    for (const y of [0, Math.round(maxScroll / 2), maxScroll]) {
      await page.evaluate(
        (top) => window.scrollTo({ top, behavior: "instant" }),
        y,
      );
      await page.waitForTimeout(250);
      const visible = await page.evaluate(() => {
        const box = document.querySelector(".sidebar").getBoundingClientRect();
        const a = document.querySelector(".sidebar-nav a.active");
        if (!a) return null;
        const r = a.getBoundingClientRect();
        return r.top >= box.top - 1 && r.bottom <= box.bottom + 1;
      });
      expect(
        visible,
        `the mark is out of sight in the sidebar at scrollY ${y}`,
      ).toBe(true);
    }
  });
});
