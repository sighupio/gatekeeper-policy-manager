import { test, expect } from '@playwright/test';

test('page constrainttemplates snapshot', async ({ page }) => {
  await page.goto('constrainttemplates/', { timeout: 10000 });
  await page.waitForSelector('span[title="Constraint Templates"]');
  await expect(page).toHaveScreenshot({ maxDiffPixels: 100, fullPage: true, mask: [page.locator('.dynamic')]  });
});