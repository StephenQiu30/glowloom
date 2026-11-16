import { test, expect } from '@playwright/test';

test('loads official knowledge base home and docs layout', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toContainText(
    '织光知识库',
  );

  const enterButton = page.getByRole('link', { name: '进入文档中心' });
  await expect(enterButton).toBeVisible();
  await enterButton.click();

  await expect(page).toHaveURL(/\/docs/);
  await expect(page.locator('article')).toBeVisible();

  const searchRes = await page.request.get('/api/search?query=需求');
  expect(searchRes.status()).toBe(200);
  const results = await searchRes.json();
  expect(Array.isArray(results)).toBe(true);
});
