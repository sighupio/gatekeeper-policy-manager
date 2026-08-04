/**
 * Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { defineConfig } from "@playwright/test";

export default defineConfig({
  use: {
    headless: true,
    browserName: "chromium",
    ignoreHTTPSErrors: true,
    // CI sets this to the port kubectl port-forward picked. Override it locally to point the
    // tests somewhere else, e.g. GPM_BASE_URL=http://192.168.2.1:8080 yarn test
    baseURL: process.env.GPM_BASE_URL ?? "http://localhost:8080",
  },
});
