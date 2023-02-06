import { test, expect } from '@playwright/test';

test('page constraints snapshot', async ({ page }) => {
  await page.goto('constraints/');
  await page.waitForSelector('span[title="Constraints"]');
  await expect(page).toHaveScreenshot({ maxDiffPixels: 100, fullPage: true, mask: [page.locator('.dynamic')] });;
});