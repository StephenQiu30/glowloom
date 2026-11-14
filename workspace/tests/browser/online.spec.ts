import { z } from 'zod';
import {
  statusContract,
  liveDocumentContract,
  runContract,
  proposalContract,
  skillsContract,
} from '../../src/online/components/contracts';
import { expect, test } from '@playwright/test';
import { randomUUID } from 'node:crypto';
test.use({
  baseURL: process.env.WORKSPACE_ONLINE_TEST_URL ?? 'http://127.0.0.1:1130',
});
function accessToken() {
  const token = process.env.WORKSPACE_ONLINE_TEST_TOKEN;
  if (!token) {
    throw new Error(
      'Configure WORKSPACE_ONLINE_TEST_TOKEN for the isolated online acceptance service',
    );
  }
  return token;
}
const origin = process.env.WORKSPACE_ONLINE_TEST_URL ?? 'http://127.0.0.1:1130';
test('keeps unauthenticated login free of private knowledge and protects every data resource', async ({
  page,
}) => {
  await page.goto('/');
  await expect(
    page.getByRole('heading', { name: '访问工作空间' }),
  ).toBeVisible();
  expect(await page.content()).not.toContain(
    'Workspace 知识库与研发工作流服务设计',
  );
  for (const path of [
    '/raw/design-43.md',
    '/pagefind/pagefind.js',
    '/narration/index.json',
    '/api/documents/',
  ]) {
    expect((await page.request.get(path)).status()).toBe(401);
  }
});
test('authenticates local ownership, rejects cross-origin writes, and preserves conflicting proposals', async ({
  page,
}) => {
  await page.goto('/login/');
  await page.getByLabel('本机访问令牌').fill(accessToken());
  await page.getByRole('button', { name: '进入工作空间' }).click();
  await expect(
    page.getByRole('heading', { name: '知识库', exact: true }),
  ).toBeVisible();
  const session = z
    .object({ csrf: z.string() })
    .parse(await (await page.request.get('/api/session/')).json());
  const headers = { origin, 'x-workspace-csrf': session.csrf };
  const status = statusContract.parse(
    await (await page.request.get('/api/status/')).json(),
  );
  expect(status.database).toBe(true);
  expect(status.bridge).toBe(true);
  expect(status.tts).toBe(false);
  expect(
    (
      await page.request.post('/api/proposals/', {
        headers: { ...headers, origin: 'https://evil.example' },
        data: {},
      })
    ).status(),
  ).toBe(403);
  const doc = liveDocumentContract.parse(
    await (await page.request.get('/api/documents?live=requirement-28')).json(),
  );
  let modified = false;
  async function create(text: string, base = doc) {
    const r = await page.request.post('/api/proposals/', {
      headers,
      data: {
        documentId: base.id,
        baseRevision: base.revision,
        baseText: base.source,
        text,
        idempotencyKey: randomUUID(),
      },
    });
    expect(r.status()).toBe(201);
    return proposalContract.parse(await r.json());
  }
  try {
    const a = await create(doc.source + '\n<!-- workspace-e2e-owned -->\n');
    const b = await create(doc.source + '\n<!-- conflicting -->\n');
    const applied = await page.request.post(`/api/proposals/${a.id}/apply/`, {
      headers,
      data: {},
    });
    expect(applied.status()).toBe(200);
    modified = true;
    const receipt = (await applied.json()).receipt;
    expect(receipt.status).toBe('applied');
    const conflict = await page.request.post(`/api/proposals/${b.id}/apply/`, {
      headers,
      data: {},
    });
    expect(conflict.status()).toBe(409);
    const replay = await page.request.post(`/api/proposals/${a.id}/apply/`, {
      headers,
      data: {},
    });
    expect((await replay.json()).receipt).toEqual(receipt);
    expect(
      (await (await page.request.get(`/api/proposals/${b.id}/`)).json())
        .receipt,
    ).toBeNull();
  } finally {
    if (modified) {
      const current = liveDocumentContract.parse(
        await (
          await page.request.get('/api/documents?live=requirement-28')
        ).json(),
      );
      const marker = '\n<!-- workspace-e2e-owned -->\n';
      expect(current.source).toContain(marker);
      const restore = await create(current.source.replace(marker, ''), current);
      expect(
        (
          await page.request.post(`/api/proposals/${restore.id}/apply/`, {
            headers,
            data: {},
          })
        ).status(),
      ).toBe(200);
    }
  }
  await page.goto('/operations/');
  await expect(page.getByRole('heading', { name: '在线协作台' })).toBeVisible();
  expect(
    (
      await page.request.post('/api/narration/generate/', { headers, data: {} })
    ).status(),
  ).toBe(403);
});
test('creates a frozen task context and verifies actual host discovery without dispatch', async ({
  page,
}) => {
  await page.goto('/login/');
  await page.getByLabel('本机访问令牌').fill(accessToken());
  await page.getByRole('button', { name: '进入工作空间' }).click();
  await expect(
    page.getByRole('heading', { name: '知识库', exact: true }),
  ).toBeVisible();
  const session = z
    .object({ csrf: z.string() })
    .parse(await (await page.request.get('/api/session/')).json());
  const headers = { origin, 'x-workspace-csrf': session.csrf };
  const created = await page.request.post('/api/runs/', {
    headers,
    data: {
      workflowId: 'feature',
      project: 'workspace',
      goal: '只读核对知识与工程入口',
      acceptance: '列出实际目录与原文版本',
      documents: ['requirement-28'],
      idempotencyKey: randomUUID(),
    },
  });
  expect(created.status()).toBe(201);
  const run = runContract.parse(await created.json());
  expect(run.status).toBe('created');
  expect(run.context.allowedActions).not.toContain('write');
  expect(run.context.stageIds).toContain('accept');
  expect(
    run.context.documents.some((d: { id: string }) => d.id === 'root-agents'),
  ).toBe(true);
  const skills = await page.request.post('/api/host/skills/', {
    headers,
    data: { project: 'workspace' },
  });
  expect(skills.status()).toBe(200);
  expect(
    skillsContract.parse(await skills.json()).filter((skill) => skill.enabled),
  ).toHaveLength(4);
  const invalid = await page.request.post(`/api/runs/${run.id}/action/`, {
    headers,
    data: {
      action: 'verify',
      commandId: 'typo',
      version: run.version,
      idempotencyKey: randomUUID(),
    },
  });
  expect(invalid.status()).toBe(400);
  const unchanged = runContract.parse(
    await (await page.request.get(`/api/runs/${run.id}/`)).json(),
  );
  expect(unchanged.status).toBe('created');
  expect(unchanged.pendingCheck ?? null).toBeNull();
  const canceled = await page.request.post(`/api/runs/${run.id}/action/`, {
    headers,
    data: {
      action: 'cancel',
      version: run.version,
      idempotencyKey: randomUUID(),
    },
  });
  expect(canceled.status()).toBe(200);
  expect((await canceled.json()).status).toBe('canceled');
});

test('uses accessible online forms and protects unsaved document edits', async ({
  page,
}) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('/login/');
  await page.getByLabel('本机访问令牌').fill('invalid-local-fixture');
  await page.getByRole('button', { name: '进入工作空间' }).click();
  await expect(page.locator('main').getByRole('alert')).toContainText(
    '访问令牌不正确',
  );
  await expect(page.getByLabel('本机访问令牌')).toBeFocused();
  await page.getByLabel('本机访问令牌').fill(accessToken());
  await page.getByRole('button', { name: '进入工作空间' }).click();
  await expect(
    page.getByRole('heading', { name: '知识库', exact: true }),
  ).toBeVisible();
  await page.goto('/operations/?document=requirement-28');
  await page.getByRole('button', { name: '读取当前原文' }).click();
  const editor = page.getByLabel('修改后的 Markdown');
  const source = await editor.inputValue();
  let releaseRead: (() => void) | undefined;
  const pendingRead = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  await page.route('**/api/documents?live=requirement-28', async (route) => {
    await pendingRead;
    await route.continue();
  });
  await page.getByRole('button', { name: '读取当前原文' }).click();
  await expect(editor).toBeDisabled();
  await expect(page.getByLabel('选择文档')).toBeDisabled();
  if (!releaseRead) {
    throw new Error('Expected a pending controlled read');
  }
  releaseRead();
  await expect(editor).toBeEnabled();
  await page.unroute('**/api/documents?live=requirement-28');

  await editor.fill(source + '\n<!-- unsaved browser-only fixture -->');
  await expect(page.getByLabel('选择文档')).toBeDisabled();
  await page.getByRole('link', { name: '知识库', exact: true }).click();
  const guard = page.getByRole('alertdialog', { name: '离开未保存的文档？' });
  await expect(guard).toBeVisible();
  await guard.getByRole('button', { name: '继续编辑' }).click();
  await expect(editor).toHaveValue(
    source + '\n<!-- unsaved browser-only fixture -->',
  );
  await page.getByRole('button', { name: '放弃本次编辑' }).click();
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: '放弃本次编辑' })
    .click();
  await expect(editor).toHaveValue(source);
  await expect(page.getByLabel('选择文档')).toBeEnabled();
  await page.getByRole('tab', { name: '任务与工作流' }).click();
  await expect(page).toHaveURL(/tab=runs/);
  await expect(page.getByLabel('任务目标')).toBeVisible();
  const session = z
    .object({ csrf: z.string() })
    .parse(await (await page.request.get('/api/session/')).json());
  const headers = { origin, 'x-workspace-csrf': session.csrf };
  const goal = `取消确认浏览器核验 ${randomUUID()}`;
  const response = await page.request.post('/api/runs/', {
    headers,
    data: {
      workflowId: 'feature',
      project: 'workspace',
      goal,
      acceptance: '只核验取消确认，不派发',
      documents: [],
      idempotencyKey: randomUUID(),
    },
  });
  expect(response.status()).toBe(201);
  const run = runContract.parse(await response.json());
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  const card = page
    .locator('[data-slot="card"]')
    .filter({ has: page.getByText(goal, { exact: true }) })
    .last();
  await card.getByRole('button', { name: '取消', exact: true }).click();
  await expect(
    page.getByRole('alertdialog', { name: '取消这个任务？' }),
  ).toBeVisible();
  expect(
    (await (await page.request.get(`/api/runs/${run.id}/`)).json()).status,
  ).toBe('created');
  await page.getByRole('button', { name: '确认取消任务' }).click();
  await expect(card.getByText('已取消', { exact: true })).toBeVisible();
  const { default: AxeBuilder } = await import('@axe-core/playwright');
  expect(
    (
      await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21aa'])
        .analyze()
    ).violations.map(({ id }) => id),
  ).toEqual([]);
  expect(errors).toEqual([]);
});
