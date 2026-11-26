import { expect, test } from '@playwright/test';

test('publishes top-level project documents with their source links', async ({
  page,
}) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toContainText(
    '织光知识库',
  );
  await page.getByRole('link', { name: '进入文档中心' }).click();
  await expect(page).toHaveURL(/\/docs$/);
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

test('preserves topic navigation and source links', async ({ page }) => {
  await page.goto('/docs');
  const article = page.locator('article');
  await article
    .getByRole('link', { name: '文档模板 (templates)', exact: true })
    .click();

  const topic = '/docs/templates/00-模板导航';
  await expect(page).toHaveURL((url) => decodeURI(url.pathname) === topic);
  await page.getByRole('button', { name: 'Open', exact: true }).click();
  await expect(
    page.getByRole('link', { name: 'Open in GitHub' }),
  ).toHaveAttribute(
    'href',
    `https://github.com/StephenQiu30/glowloom/blob/main/workspace/content/${encodeURI('templates/00-模板导航.md')}`,
  );
  await expect(
    page.getByRole('link', { name: 'View as Markdown' }),
  ).toHaveAttribute('href', encodeURI(`${topic}.md`));
  await page.keyboard.press('Escape');
  await article
    .getByRole('link', { name: '返回文档中心', exact: true })
    .click();
  await expect(page).toHaveURL(/\/docs$/);
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
