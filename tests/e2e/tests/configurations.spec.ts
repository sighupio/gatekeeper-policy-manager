import { test, expect } from '@playwright/test';

test('page configurations snapshot', async ({ page }) => {
  await page.goto('configurations/');
  await page.waitForSelector('#config > .euiPanel');
  await expect(page).toHaveScreenshot({ maxDiffPixels: 100, fullPage: true, mask: [page.locator('.dynamic')]  });
});