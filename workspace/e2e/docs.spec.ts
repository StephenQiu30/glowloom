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
  expect(results.length).toBeGreaterThan(0);
});

test('keeps document routes and source links after flattening content', async ({
  page,
}) => {
  await page.goto('/docs');
  await page
    .locator('article')
    .getByRole('link', { name: '文档模板 (templates)', exact: true })
    .click();

  const documentPath = '/docs/templates/00-模板导航';
  await expect(page).toHaveURL(
    (url) => decodeURI(url.pathname) === documentPath,
  );
  await expect(page.locator('article')).toContainText('模板');

  await page.getByRole('button', { name: 'Open', exact: true }).click();
  await expect(
    page.getByRole('link', { name: 'Open in GitHub' }),
  ).toHaveAttribute(
    'href',
    `https://github.com/StephenQiu30/glowloom/blob/main/workspace/content/${encodeURI('templates/00-模板导航.md')}`,
  );
  await expect(
    page.getByRole('link', { name: 'View as Markdown' }),
  ).toHaveAttribute('href', encodeURI(`${documentPath}.md`));

  const markdown = await page.request.get(`${documentPath}.md`);
  expect(markdown.status()).toBe(200);
  expect(await markdown.text()).toContain('模板');

  await page.keyboard.press('Escape');
  await page
    .locator('article')
    .getByRole('link', { name: '返回文档中心', exact: true })
    .click();
  await expect(page).toHaveURL(/\/docs$/);

  const search = await page.request.get('/api/search?query=模板');
  expect(search.status()).toBe(200);
  const results: { url: string }[] = await search.json();
  const template = results.find((result) =>
    decodeURI(result.url).startsWith('/docs/templates/'),
  );
  expect(template).toBeDefined();
  await page.goto(template!.url);
  await expect(page.locator('article')).toBeVisible();
});
