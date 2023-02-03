import { test, expect } from '@playwright/test';

test('page configurations snapshot', async ({ page }) => {
  await page.goto('http://localhost:8080/configurations/');
  await page.waitForSelector('#config > .euiPanel');
  await expect(page).toHaveScreenshot({ maxDiffPixels: 100, fullPage: true });
});