import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test('restores search state from URL and keyboard focus from the command palette', async ({
  page,
}) => {
  await page.goto('/search/?q=FR-WS-01&category=产品需求');
  await expect(page.getByRole('searchbox', { name: '全文搜索' })).toHaveValue(
    'FR-WS-01',
  );
  await expect(page.getByLabel('文档分类')).toHaveValue('产品需求');
  await expect(page.locator('.search-result').first()).toBeVisible();
  await page.getByLabel('文档分类').selectOption('设计方案');
  await expect(page).toHaveURL(/category=/);
  await page.goBack();
  await expect(page.getByLabel('文档分类')).toHaveValue('产品需求');
  await page.reload();
  await expect(page.getByRole('searchbox', { name: '全文搜索' })).toHaveValue(
    'FR-WS-01',
  );
  const trigger = page.getByRole('button', { name: '搜索知识库', exact: true });
  await trigger.focus();
  await page.keyboard.press('Control+k');
  await expect(page.getByRole('dialog', { name: '搜索知识库' })).toBeVisible();
  await page.getByPlaceholder('搜索文档标题…').fill('Workspace');
  await expect(page.getByRole('option').first()).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(trigger).toBeFocused();
});

for (const theme of ['浅色', '深色']) {
  test(`uses accessible semantic components in ${theme}`, async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.goto('/');
    await page.getByRole('button', { name: '切换主题' }).click();
    await page.getByRole('menuitemradio', { name: theme, exact: true }).click();
    await expect(page.locator('html')).toHaveClass(
      theme === '深色' ? /dark/ : /light/,
    );
    for (const path of [
      '/',
      '/read/prd-29/',
      '/search/',
      '/skills/',
      '/workflows/',
      '/tasks/',
      '/engineering/',
      '/graph/',
    ]) {
      await page.goto(path);
      await expect(page.locator('h1')).toBeVisible();
      const audit = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21aa'])
        .analyze();
      expect(
        audit.violations.map(({ id, nodes }) => ({
          id,
          targets: nodes.map((node) => node.target),
        })),
        path,
      ).toEqual([]);
    }
    expect(errors).toEqual([]);
  });
}

test('keeps small screens and landscape usable with reduced motion and large text', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  for (const viewport of [
    { width: 375, height: 812 },
    { width: 812, height: 375 },
    { width: 768, height: 1024 },
  ]) {
    await page.setViewportSize(viewport);
    for (const path of ['/', '/read/design-43/', '/tasks/', '/graph/']) {
      await page.goto(path);
      await page.addStyleTag({ content: 'html {font-size: 20px;}' });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
        `${path} at ${viewport.width}px`,
      ).toBe(true);
    }
  }
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto('/search/');
  expect(
    await page
      .getByRole('searchbox', { name: '全文搜索' })
      .evaluate((element) => parseFloat(getComputedStyle(element).fontSize)),
  ).toBeGreaterThanOrEqual(16);
  for (const name of ['打开项目目录', '搜索知识库', '切换主题']) {
    const bounds = await page
      .getByRole('button', { name, exact: true })
      .boundingBox();
    expect(bounds?.height, name).toBeGreaterThanOrEqual(44);
    expect(bounds?.width, name).toBeGreaterThanOrEqual(44);
  }
});

test('collapses the local shadcn sidebar to icons and keeps document navigation available', async ({
  page,
}) => {
  await page.goto('/');
  const collapse = page.getByRole('button', { name: '折叠侧栏', exact: true });
  await collapse.click();
  await expect(
    page.locator('[data-slot="sidebar"][data-collapsible="icon"]'),
  ).toBeVisible();
  await expect(page.getByLabel('筛选目录')).not.toBeVisible();
  await page.getByRole('button', { name: '展开侧栏', exact: true }).click();
  await expect(page.getByLabel('筛选目录')).toBeVisible();
  await page.getByLabel('筛选目录').fill('Workspace');
  const document = page
    .getByRole('navigation', { name: '项目目录' })
    .locator('a[href="/read/design-43/"]');
  await document.click();
  await expect(
    page.getByRole('heading', {
      name: 'Workspace 知识库与研发工作流服务设计',
      exact: true,
    }),
  ).toBeVisible();
  await expect(document).toHaveAttribute('aria-current', 'page');
});

test('uses the framework tree, heading observer and adjacent document links without changing old routes', async ({
  page,
}) => {
  await page.goto('/read/doc-e75d48f9905e/');
  const directory = page.getByRole('navigation', { name: '项目目录' });
  await expect(
    directory.locator('a[href="/read/doc-e75d48f9905e/"]'),
  ).toHaveAttribute('aria-current', 'page');
  await page.goto('/read/design-43/');
  const outline = page
    .locator('aside.toc')
    .getByRole('navigation', { name: '本页目录' });
  const link = outline
    .getByRole('link')
    .filter({ hasText: '7.1 决策与权威归属' });
  await link.click();
  await expect(page).toHaveURL(/#7[.]1-|#71-/);
  await expect(link).toHaveAttribute('data-active', 'true');
  const next = page
    .getByRole('navigation', { name: '相邻文档' })
    .getByRole('link', { name: /^下一篇/ });
  const href = await next.getAttribute('href');
  expect(href).toMatch(/^\/read\/.+\/$/);
  await next.click();
  await expect(page).toHaveURL((url) => url.pathname === href);
  await page.goto('/read/root-readme/');
  await expect(page.locator('main h1')).toHaveCount(1);
});
