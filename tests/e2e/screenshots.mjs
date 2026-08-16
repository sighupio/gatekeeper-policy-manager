// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Captures the README screenshots from a running GPM. Point it at a GPM that already has some
// Gatekeeper objects to show (the e2e cluster is ideal) via GPM_BASE_URL, then run it with
// `mise run gen:screenshots`. The "-02" shots open the first card so the rego and the violations
// table are visible.

import { chromium } from "playwright";
import { mkdir } from "node:fs/promises";

const base = process.env.GPM_BASE_URL ?? "http://localhost:8080";
const outDir = "../../screenshots";

const shots = [
  { file: "home.png", path: "/" },
  { file: "constraint-templates-01.png", path: "/constrainttemplates" },
  { file: "constraint-templates-02.png", path: "/constrainttemplates", expandFirstCard: true },
  { file: "constraints-01.png", path: "/constraints" },
  { file: "constraints-02.png", path: "/constraints", expandFirstCard: true },
  { file: "violations-report.png", path: "/constraints?report=html" },
  { file: "mutations.png", path: "/mutations" },
  { file: "events.png", path: "/events" },
  { file: "configurations.png", path: "/configurations" },
];

await mkdir(outDir, { recursive: true });
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

for (const s of shots) {
  await page.goto(base + s.path, { waitUntil: "networkidle" });
  if (s.expandFirstCard) {
    // Open every <details> in the first card so its rego / violations table shows.
    await page.locator(".card").first().locator("details").evaluateAll((els) => els.forEach((d) => (d.open = true)));
    await page.waitForTimeout(200);
  }
  await page.screenshot({ path: `${outDir}/${s.file}`, fullPage: true });
  console.log("captured", s.file);
}

await browser.close();
