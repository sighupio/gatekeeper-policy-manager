import { defineConfig } from "@playwright/test";

export default defineConfig({
    use: {
        headless: true,
        browserName: "chromium",
        ignoreHTTPSErrors: true,
        baseURL: "http://localhost:8080",
    },
});
