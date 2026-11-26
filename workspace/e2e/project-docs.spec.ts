import { expect, test } from '@playwright/test';

test('publishes top-level project documents with their source links', async ({
  page,
}) => {
  await page.goto('/docs');
  const sidebar = page.locator('#nd-sidebar');
  const documents = [
    {
      path: '/docs/architecture',
      name: '织光总体架构设计',
      file: 'ARCHITECTURE.md',
    },
    {
      path: '/docs/project',
      name: '织光项目工程规范',
      file: 'PROJECT.md',
    },
  ];

  for (const { path, name, file } of documents) {
    await sidebar.getByRole('link', { name: new RegExp(name) }).click();
    await expect(page).toHaveURL(new RegExp(`${path}$`));
    await expect(
      page.locator('article').getByRole('heading', { level: 1 }),
    ).toContainText(name);

    await page.getByRole('button', { name: 'Open', exact: true }).click();
    await expect(
      page.getByRole('link', { name: 'Open in GitHub' }),
    ).toHaveAttribute(
      'href',
      `https://github.com/StephenQiu30/glowloom/blob/main/workspace/${file}`,
    );
    await page.keyboard.press('Escape');

    const search = await page.request.get('/api/search', {
      params: { query: name },
    });
    expect(search.ok()).toBe(true);
    expect(await search.json()).toEqual(
      expect.arrayContaining([expect.objectContaining({ url: path })]),
    );

    const markdown = await page.request.get(`${path}.md`);
    expect(markdown.ok()).toBe(true);
    expect(await markdown.text()).toContain(name);
  }
});

test('opens the project knowledge flow from the architecture page', async ({
  page,
}) => {
  await page.goto('/docs/architecture');

  await page
    .locator('article')
    .getByRole('link', { name: 'Archify · 资料入库、版本发布与撤回' })
    .click();

  await expect(page).toHaveURL(
    /\/design\/project-knowledge\/knowledge-intake\.html$/,
  );
  await expect(
    page.getByRole('button', {
      name: /^聚焦确认并原子发布，重检权限及候选版本/,
    }),
  ).toBeVisible();
});

test('uses the official Spacious reader and theme switch', async ({ page }) => {
  await page.goto('/docs/architecture');
  await expect(page.locator('#nd-page-panel article')).toBeVisible();

  const options = page
    .locator('#nd-page-panel')
    .getByRole('button', { name: 'Options', exact: true });
  await options.click();
  await page.getByRole('menuitemradio', { name: 'Dark', exact: true }).click();
  await expect(page.locator('html')).toHaveClass(/\bdark\b/);

  await page.reload();
  await expect(page.locator('html')).toHaveClass(/\bdark\b/);
  await options.click();
  await page.getByRole('menuitemradio', { name: 'Light', exact: true }).click();
  await expect(page.locator('html')).toHaveClass(/\blight\b/);
});
