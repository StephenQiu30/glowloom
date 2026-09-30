import { expect, test, type Page } from "@playwright/test";

// 仅在本轮隔离数据库和合成账号已准备后显式启用；所有请求走正式 API。
const enabled = process.env.LV_E2E_PROJECT_CREATE === "1";
const loginName = process.env.LV_E2E_LOGIN_NAME;
const initialPassword = process.env.LV_E2E_PASSWORD;
const readyPassword = process.env.LV_E2E_READY_PASSWORD;
test.skip(
  !enabled || !loginName || !initialPassword || !readyPassword,
  "需要显式启用项目创建验证并配置隔离合成账号",
);
test.setTimeout(90_000);

async function login(page: Page, password: string) {
  await page.goto("/login?returnTo=%2Fprojects");
  await page.getByLabel("账号", { exact: true }).fill(loginName!);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect
    .poll(
      async () =>
        (await page
          .getByRole("heading", { name: "修改初始密码" })
          .isVisible()) || new URL(page.url()).pathname === "/projects",
    )
    .toBe(true);
  if (await page.getByRole("heading", { name: "修改初始密码" }).isVisible()) {
    await page.getByLabel("当前密码", { exact: true }).fill(password);
    await page.getByLabel("新密码", { exact: true }).fill(readyPassword!);
    await page
      .getByLabel("再次输入新密码", { exact: true })
      .fill(readyPassword!);
    await page.getByRole("button", { name: "更新密码", exact: true }).click();
  }
  await expect(page).toHaveURL(/\/projects$/);
  await expect(
    page.getByRole("button", { name: "新建项目", exact: true }),
  ).toBeVisible();
}

async function noPageOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
}

test.beforeAll(async ({ browser }) => {
  const page = await browser.newPage();
  try {
    await login(page, initialPassword!);
  } finally {
    await page.close();
  }
});
test.beforeEach(async ({ page }) => {
  await login(page, readyPassword!);
});

test("空项目列表创建正式项目，再保存画布并刷新", async ({ page }) => {
  await expect(
    page.getByText("当前列表没有项目。", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "新建项目", exact: true }).click();
  const name = `正式项目验证-${crypto.randomUUID().slice(0, 8)}`;
  await page.getByLabel("项目名称", { exact: true }).fill(name);
  await page
    .getByLabel("描述（可选）", { exact: true })
    .fill("从项目创建进入无限画布");
  await page.getByRole("radio", { name: "风格化", exact: true }).click();
  await page.getByRole("radio", { name: "国风仙侠", exact: true }).click();
  const responsePromise = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/projects" &&
      response.request().method() === "POST",
  );
  await page
    .getByRole("button", { name: "创建并进入画布", exact: true })
    .click();
  const response = await responsePromise;
  expect(response.status()).toBe(201);
  const project = await response.json();
  expect(project).toMatchObject({
    name,
    description: "从项目创建进入无限画布",
    aspect_ratio: "16:9",
    style_type: "stylized",
    style_subtype: "guofeng_xianxia",
    resolution: "1080p",
    allow_overseas_models: false,
    status: "active",
    revision: 1,
  });
  expect(project.id).toMatch(/^[0-9a-f-]{36}$/);
  expect(response.request().headers()["idempotency-key"]).toMatch(
    /^[0-9a-f-]{36}$/,
  );
  await expect(page).toHaveURL(new RegExp(`/projects/${project.id}/canvas$`));
  await expect(
    page.getByRole("heading", { name: "无限画布", exact: true }),
  ).toBeVisible();
  await page.getByLabel("新画布名称", { exact: true }).fill("项目首张画布");
  await page.getByRole("button", { name: "创建画布", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
  const canvasId = new URL(page.url()).searchParams.get("canvas");
  expect(canvasId).toMatch(/^[0-9a-f-]{36}$/);
  async function readCanvas() {
    const result = await page.request.get(`/api/canvases/${canvasId}`);
    expect(result.status()).toBe(200);
    return result.json();
  }
  let document = await readCanvas();
  await page.getByRole("button", { name: "文字", exact: true }).click();
  await expect.poll(async () => (await readCanvas()).nodes.length).toBe(1);
  document = await readCanvas();
  await page
    .locator(`[data-node-id="${document.nodes[0].id}"]`)
    .click({ position: { x: 20, y: 20 } });
  await page
    .getByLabel("文字内容", { exact: true })
    .fill("项目与无限画布真实保存链");
  await page.getByRole("button", { name: "保存文字", exact: true }).click();
  await expect
    .poll(async () => (await readCanvas()).nodes[0].config.text)
    .toBe("项目与无限画布真实保存链");
  document = await readCanvas();
  await expect(
    page
      .getByRole("status")
      .filter({ hasText: `已保存 · 修订 ${document.revision}` }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
  expect(await readCanvas()).toEqual(document);
  await expect(
    page.getByText("项目与无限画布真实保存链", { exact: true }).first(),
  ).toBeVisible();
  await noPageOverflow(page);
  await page.screenshot({
    path: "/tmp/lanverse-project-canvas-desktop-20260930.png",
  });
});

test("390px 的正式项目列表和创建表单可使用", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await noPageOverflow(page);
  await page.getByRole("button", { name: "新建项目", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "新建项目", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("dialog", { name: "新建项目", exact: true })
    .evaluate(async (element) => {
      await Promise.all(
        element
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished.catch(() => undefined)),
      );
    });
  await page
    .getByLabel("项目名称", { exact: true })
    .fill(`移动项目-${crypto.randomUUID().slice(0, 8)}`);
  await noPageOverflow(page);
  await page.screenshot({
    path: "/tmp/lanverse-project-create-mobile-20260930.png",
  });
  const responsePromise = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/projects" &&
      response.request().method() === "POST",
  );
  await page
    .getByRole("button", { name: "创建并进入画布", exact: true })
    .click();
  const response = await responsePromise;
  expect(response.status()).toBe(201);
  const project = await response.json();
  expect(project).toMatchObject({
    style_type: "realistic",
    resolution: "1080p",
    allow_overseas_models: false,
    revision: 1,
  });
  expect(project.style_subtype).toBeUndefined();
  await expect(page).toHaveURL(new RegExp(`/projects/${project.id}/canvas$`));
  await expect(
    page.getByRole("heading", { name: "无限画布", exact: true }),
  ).toBeVisible();
  await noPageOverflow(page);
});

test("搜索、状态和表格视图读取真实项目", async ({ page }) => {
  const response = await page.request.get("/api/projects?limit=20");
  expect(response.status()).toBe(200);
  const { items } = await response.json();
  expect(items.length).toBeGreaterThan(0);
  const project = items[0];
  await page.getByLabel("搜索项目", { exact: true }).fill(project.name);
  const searchResponse = page.waitForResponse((result) => {
    const url = new URL(result.url());
    return (
      url.pathname === "/api/projects" &&
      url.searchParams.get("q") === project.name
    );
  });
  await page.getByRole("button", { name: "搜索", exact: true }).click();
  const matched = await searchResponse;
  expect(matched.status()).toBe(200);
  expect(
    (await matched.json()).items.map((item: { id: string }) => item.id),
  ).toContain(project.id);
  await expect(
    page.getByRole("link", { name: project.name, exact: true }),
  ).toBeVisible();
  await page.getByRole("radio", { name: "已归档", exact: true }).click();
  await expect(
    page.getByText("当前列表没有项目。", { exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(/status=archived/);
  await page.getByRole("radio", { name: "进行中", exact: true }).click();
  await expect(
    page.getByRole("link", { name: project.name, exact: true }),
  ).toBeVisible();
  await page.getByRole("radio", { name: "表格视图", exact: true }).click();
  await expect(page.getByRole("table")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await noPageOverflow(page);
  await page.getByRole("link", { name: project.name, exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${project.id}/canvas$`));
  await expect(
    page.getByRole("heading", { name: "无限画布", exact: true }),
  ).toBeVisible();
});
