import { expect, test } from '@playwright/test';

test('reads the existing documents through the official layout without login', async ({
  page,
}) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/read\/?$/);
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  await expect(
    page.getByRole('link', { name: '设计43', exact: true }).first(),
  ).toHaveAttribute('href', '/read/design-43');
  await page.goto('/read/design-43');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(
    '43 Workspace 知识库与研发工作流服务设计',
  );
  await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
  await expect(page.getByLabel('本机访问令牌')).toHaveCount(0);
  await expect(page.locator('#nd-sidebar')).toBeVisible();
  await expect(page.locator('svg[aria-roledescription]').first()).toBeVisible({
    timeout: 15000,
  });
  expect((await page.request.get('/read/design-43.md')).status()).toBe(200);
  expect((await page.request.get('/raw/design-43.md')).status()).toBe(200);
  expect(
    (
      await page.request.get(
        '/docs/design/43-Workspace知识库与研发工作流服务设计.md',
      )
    ).status(),
  ).toBe(200);
  expect((await page.request.get('/llms.txt')).status()).toBe(200);
  expect((await page.request.get('/llms-full.txt')).status()).toBe(200);
  expect((await page.request.get('/operations/')).status()).toBe(404);
  expect((await page.request.post('/api/runs/', { data: {} })).status()).toBe(
    404,
  );
});

test('uses the official Meilisearch dialog and document links', async ({
  page,
}) => {
  await page.goto('/read/design-43');
  await page.keyboard.press('Control+k');
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await page.getByRole('combobox', { name: '搜索文档' }).fill('FR-WS-01');
  const result = dialog
    .getByRole('option')
    .filter({ hasText: 'FR-WS-01' })
    .first();
  await expect(result).toBeVisible();
  await result.click();
  await expect(dialog).not.toBeVisible();
  await expect(page).toHaveURL(/\/read\/[^/]+/);
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
});

test('keeps the official docs usable on mobile', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/read/design-43');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  await page.getByRole('button', { name: 'Open Sidebar', exact: true }).click();
  const sidebar = page.locator('#nd-sidebar-mobile');
  await expect(sidebar).toHaveAttribute('data-state', 'open');
  await expect(sidebar).toBeVisible();
  await sidebar
    .getByRole('button', { name: 'Toggle Theme', exact: true })
    .click();
  await expect(page.locator('html')).toHaveClass(/dark/);
  await sidebar
    .getByRole('button', { name: 'Toggle Theme', exact: true })
    .click();
  await expect(page.locator('html')).toHaveClass(/light/);
  await sidebar
    .getByRole('button', { name: 'Close Sidebar', exact: true })
    .click();
  await expect(sidebar).toHaveAttribute('data-state', 'closed');
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
});

test('organizes numbered knowledge and excludes tool process documents', async ({
  page,
}) => {
  await page.goto('/read/doc-e75d48f9905e');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(
    '01 腾讯云 COS 对象存储',
  );
  await expect(page.locator('#nd-sidebar')).toContainText('部署运维');
  await expect(page.locator('#nd-sidebar')).not.toContainText('Superpowers');
  expect((await page.request.get('/docs/COS配置说明.md')).status()).toBe(200);
  for (const id of ['doc-aced3262dad7', 'doc-4582c43a53c2']) {
    expect((await page.request.get(`/read/${id}`)).status()).toBe(404);
    expect((await page.request.get(`/raw/${id}.md`)).status()).toBe(404);
  }
  const index = await (await page.request.get('/static.json')).json();
  expect(
    index.some((entry: { url: string }) =>
      /doc-aced3262dad7|doc-4582c43a53c2/.test(entry.url),
    ),
  ).toBe(false);
});

test('shows project branding in navigation and browser icons', async ({
  page,
}) => {
  await page.goto('/read');
  const logo = page.locator('img[src="/brand/logo.svg"]:visible').first();
  await expect(logo).toBeVisible();
  await expect(logo).toHaveJSProperty('naturalWidth', 32);
  await expect(page.locator('link[rel="icon"][href^="/icon.svg"]')).toHaveCount(
    1,
  );
  await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveCount(1);
  for (const path of [
    '/brand/logo.svg',
    '/icon.svg',
    '/favicon.ico',
    '/apple-icon.png',
  ]) {
    expect((await page.request.get(path)).status()).toBe(200);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(logo).toBeVisible();
});
