/**
 * Copyright (c) 2026 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { test, expect } from "@playwright/test";

// The violations table renders its Message cells with linkifyInto (issue #1325), the client-side
// twin of linkify in ssr.go. The e2e cluster's policies emit no URLs, so a pixel snapshot cannot
// cover it. This drives the real shipped asset on the real page instead, under the page's strict
// Content Security Policy.
declare function linkifyInto(el: Element, msg: string): void;

test("violation messages render URLs as links, and never as markup", async ({
  page,
}) => {
  await page.goto("constraints/");

  const out = await page.evaluate(() => {
    const render = (msg: string) => {
      const el = document.createElement("td");
      linkifyInto(el, msg);
      const a = el.querySelector("a");
      return {
        text: el.textContent,
        href: a?.getAttribute("href") ?? null,
        target: a?.getAttribute("target") ?? null,
        rel: a?.getAttribute("rel") ?? null,
        elements: el.querySelectorAll("*").length,
      };
    };
    return {
      link: render(
        "Rejecting Pod. See https://docs.sighup.io/policy for more details.",
      ),
      markup: render('Rejecting <img src=x onerror="alert(1)"> for no probe'),
    };
  });

  // The URL becomes a link; the sentence's trailing period stays outside it.
  expect(out.link.href).toBe("https://docs.sighup.io/policy");
  expect(out.link.target).toBe("_blank");
  expect(out.link.rel).toBe("noopener noreferrer");
  expect(out.link.text).toBe(
    "Rejecting Pod. See https://docs.sighup.io/policy for more details.",
  );

  // A message that looks like markup stays text: no element is built from it.
  expect(out.markup.elements).toBe(0);
  expect(out.markup.text).toBe(
    'Rejecting <img src=x onerror="alert(1)"> for no probe',
  );
});
