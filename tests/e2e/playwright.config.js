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
        baseURL: "http://ramiros-mbp.milano.algozino.com.ar:8080",
    },
});
