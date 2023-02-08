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
        baseURL: "http://localhost:8080",
    },
});
