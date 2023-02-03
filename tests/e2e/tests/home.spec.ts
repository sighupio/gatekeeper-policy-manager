import { test, expect } from '@playwright/test';

test('page home snapshot', async ({ page }) => {
  await page.goto('http://localhost:8080');
  await expect(page).toHaveScreenshot({ maxDiffPixels: 100, fullPage: true });
});