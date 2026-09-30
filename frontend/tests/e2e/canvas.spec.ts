import { expect, test, type Locator, type Page } from "@playwright/test";

// 必须指向本轮隔离 HTTP fixture；不创建默认生产账号，不使用 mock 路由。
const projectId = process.env.LV_E2E_PROJECT_ID;
const loginName = process.env.LV_E2E_LOGIN_NAME ?? "canvas-browser";
const initialPassword = process.env.LV_E2E_PASSWORD ?? "CanvasTest123!";
const readyPassword = "CanvasReady123!";
const baseURL = process.env.LV_E2E_BASE_URL ?? "http://127.0.0.1:3140";
type Document = {
  id: string;
  revision: number;
  nodes: {
    id: string;
    node_type: string;
    title: string;
    x: number;
    y: number;
    width: number;
    height: number;
    parent_id?: string;
    config: { text?: string; collapsed?: boolean };
    ref_id?: string;
  }[];
  edges: { id: string; source_node_id: string; target_node_id: string }[];
  viewport: { x: number; y: number; zoom: number };
};
test.skip(!projectId, "需要 LV_E2E_PROJECT_ID 指向隔离真实 API 项目");
test.setTimeout(90_000);

async function login(page: Page, password: string) {
  await page.goto(
    `${baseURL}/login?returnTo=${encodeURIComponent(`/projects/${projectId}/canvas`)}`,
  );
  await page.getByLabel("账号", { exact: true }).fill(loginName);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect
    .poll(
      async () =>
        (await page
          .getByRole("heading", { name: "修改初始密码" })
          .isVisible()) || new URL(page.url()).pathname.endsWith("/canvas"),
    )
    .toBe(true);
  if (await page.getByRole("heading", { name: "修改初始密码" }).isVisible()) {
    await expect(page).toHaveURL(/\/login\?/);
    await page.getByLabel("当前密码", { exact: true }).fill(password);
    await page.getByLabel("新密码", { exact: true }).fill(readyPassword);
    await page
      .getByLabel("再次输入新密码", { exact: true })
      .fill(readyPassword);
    await page.getByRole("button", { name: "更新密码", exact: true }).click();
  }
  await expect(
    page.getByRole("heading", { name: "无限画布", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/canvas`));
}
async function create(page: Page) {
  await page
    .getByLabel("新画布名称", { exact: true })
    .fill(`画布浏览器验证-${crypto.randomUUID().slice(0, 8)}`);
  await page.getByRole("button", { name: "创建画布", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
  const id = new URL(page.url()).searchParams.get("canvas");
  expect(id).toMatch(/^[0-9a-f-]{36}$/);
  return id!;
}
async function read(page: Page, id: string): Promise<Document> {
  const response = await page.request.get(`/api/canvases/${id}`);
  expect(response.status()).toBe(200);
  return response.json();
}
async function saved(page: Page, id: string, previous: number) {
  await expect
    .poll(async () => (await read(page, id)).revision)
    .toBeGreaterThan(previous);
  const doc = await read(page, id);
  await expect(
    page
      .getByRole("status")
      .filter({ hasText: `已保存 · 修订 ${doc.revision}` }),
  ).toBeVisible();
  return doc;
}
async function drag(page: Page, target: Locator, dx: number, dy: number) {
  const rect = await target.boundingBox();
  expect(rect).not.toBeNull();
  await page.mouse.move(rect!.x + rect!.width / 2, rect!.y + rect!.height / 2);
  await page.mouse.down();
  await page.mouse.move(
    rect!.x + rect!.width / 2 + dx,
    rect!.y + rect!.height / 2 + dy,
    { steps: 8 },
  );
  await page.mouse.up();
}
test.beforeAll(async ({ browser }) => {
  const page = await browser.newPage();
  try {
    await login(page, initialPassword);
  } finally {
    await page.close();
  }
});
test.beforeEach(async ({ page }) => {
  await login(page, readyPassword);
});

test("文字、连线、分组、复制、撤销及刷新走同一正式保存链", async ({ page }) => {
  const id = await create(page);
  let doc = await read(page, id);
  await page.getByRole("button", { name: "文字", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  const firstId = doc.nodes[0].id;
  const first = page.locator(`[data-node-id="${firstId}"]`);
  await first.click({ position: { x: 20, y: 20 } });
  await page
    .getByLabel("文字内容", { exact: true })
    .fill("无限画布正式保存验证");
  await page.getByRole("button", { name: "保存文字", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  await drag(page, first.locator("header"), -460, -30);
  doc = await saved(page, id, doc.revision);
  await page.getByRole("button", { name: "文字", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  const secondId = doc.nodes.find((node) => node.id !== firstId)!.id;
  const source = first.locator('[data-node-handle="source"]');
  const target = page.locator(
    `[data-node-id="${secondId}"] [data-node-handle="target"]`,
  );
  const start = await source.boundingBox(),
    end = await target.boundingBox();
  expect(start).not.toBeNull();
  expect(end).not.toBeNull();
  await page.mouse.move(
    start!.x + start!.width / 2,
    start!.y + start!.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(end!.x + end!.width / 2, end!.y + end!.height / 2, {
    steps: 8,
  });
  await page.mouse.up();
  doc = await saved(page, id, doc.revision);
  expect(doc.edges).toHaveLength(1);
  await page.keyboard.press("ControlOrMeta+a");
  await page.getByRole("button", { name: "分组", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  expect(doc.nodes).toHaveLength(3);
  const group = doc.nodes.find((node) => node.node_type === "group")!;
  expect(doc.nodes.filter((node) => node.parent_id === group.id)).toHaveLength(
    2,
  );
  await page.getByRole("button", { name: "撤销", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  expect(doc.nodes).toHaveLength(2);
  await page.getByRole("button", { name: "重做", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  await page
    .locator(`[data-node-id="${group.id}"]`)
    .click({ position: { x: 12, y: 12 } });
  await page.getByRole("button", { name: "复制", exact: true }).click();
  await page.getByRole("button", { name: "粘贴", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  expect(doc.nodes).toHaveLength(6);
  expect(new Set(doc.nodes.map((node) => node.id)).size).toBe(6);
  expect(doc.edges).toHaveLength(2);
  await page.getByRole("button", { name: "缩小画布", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  expect(doc.viewport.zoom).toBeLessThan(1);
  await page.reload();
  await expect(
    page.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
  expect(await read(page, id)).toEqual(doc);
  await expect(
    page.getByRole("button", { name: "撤销", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByText("无限画布正式保存验证", { exact: true }).first(),
  ).toBeVisible();
});

test("两页修订冲突保留文字草稿，读取最新后恢复编辑", async ({
  page,
  context,
}) => {
  const id = await create(page);
  let doc = await read(page, id);
  await page.getByRole("button", { name: "文字", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  const nodeId = doc.nodes[0].id,
    other = await context.newPage();
  await other.goto(page.url());
  await expect(
    other.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
  for (const [tab, content] of [
    [page, "先保存的内容"],
    [other, "冲突页保留的草稿"],
  ] as const) {
    await tab
      .locator(`[data-node-id="${nodeId}"]`)
      .click({ position: { x: 20, y: 20 } });
    await tab.getByLabel("文字内容", { exact: true }).fill(content);
  }
  await page.getByRole("button", { name: "保存文字", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  await other.getByRole("button", { name: "保存文字", exact: true }).click();
  await expect(
    other.getByRole("alert").filter({ hasText: "已被其他页面修改" }),
  ).toBeVisible();
  await expect(other.getByLabel("文字内容", { exact: true })).toHaveValue(
    "冲突页保留的草稿",
  );
  other.once("dialog", (dialog) => dialog.accept());
  await other
    .getByRole("button", { name: "放弃并读取最新", exact: true })
    .click();
  await expect(
    other.getByText("先保存的内容", { exact: true }).first(),
  ).toBeVisible();
  expect((await read(other, id)).revision).toBe(doc.revision);
  await other.close();
});

test("项目媒体引用通过真实 MinIO 预览，图像可见且视频音频可播放", async ({
  page,
}) => {
  const id = await create(page);
  let doc = await read(page, id);
  for (const [name, kind] of [
    ["Canvas-owned-image.png", "image"],
    ["Canvas-owned-video.mp4", "video"],
    ["Canvas-owned-audio.wav", "audio"],
  ] as const) {
    await page.getByRole("button", { name: "媒体库", exact: true }).click();
    await page
      .getByRole("button", { name: `${name} · ${kind}`, exact: true })
      .click();
    doc = await saved(page, id, doc.revision);
    const node = doc.nodes.find((node) => node.title === name)!;
    expect(node.ref_id).toMatch(/^[0-9a-f-]{36}$/);
    const element = page.locator(`[data-node-id="${node.id}"]`);
    await element.click({ position: { x: 20, y: 20 } });
    await expect(
      element.locator(kind === "image" ? "img" : kind),
    ).toHaveAttribute("crossorigin", "anonymous");
    if (kind === "image")
      await expect
        .poll(async () =>
          element
            .locator("img")
            .evaluate((image) => (image as HTMLImageElement).naturalWidth),
        )
        .toBe(160);
    else {
      const player = element.locator(kind);
      await expect
        .poll(async () =>
          player.evaluate((media) => (media as HTMLMediaElement).readyState),
        )
        .toBeGreaterThanOrEqual(1);
      await player.evaluate((media) => (media as HTMLMediaElement).play());
      await expect
        .poll(async () =>
          player.evaluate((media) => (media as HTMLMediaElement).currentTime),
        )
        .toBeGreaterThan(0);
      await player.evaluate((media) => (media as HTMLMediaElement).pause());
      await expect(page.locator("video[src],audio[src]")).toHaveCount(1);
    }
    if (kind !== "audio") {
      await drag(page, element.locator("header"), -250, -30);
      doc = await saved(page, id, doc.revision);
    }
  }
  expect(doc.nodes.map((node) => node.node_type).sort()).toEqual([
    "audio",
    "image",
    "video",
  ]);
  await page.keyboard.press("ControlOrMeta+a");
  await expect(page.locator("video[src],audio[src]")).toHaveCount(0);
  const videoNode = doc.nodes.find((node) => node.node_type === "video")!;
  await page.locator(`[data-node-id="${videoNode.id}"]`).hover();
  const hover = page.locator("video[data-canvas-hover-preview]");
  await expect(hover).toHaveCount(1);
  await expect(hover).toHaveAttribute("crossorigin", "anonymous");
  await expect
    .poll(() =>
      hover.evaluate((media) => (media as HTMLMediaElement).currentTime),
    )
    .toBeGreaterThan(0);
  const bounds = (await page
    .getByRole("region", { name: "画布编辑区", exact: true })
    .boundingBox())!;
  await page.mouse.move(bounds.x + 12, bounds.y + 12);
  await expect(hover).toHaveCount(0);
});

test("物理缩放节点、重命名、平移框选及分组折叠保存正式状态", async ({
  page,
}) => {
  const id = await create(page);
  let doc = await read(page, id);
  await page.getByRole("button", { name: "文字", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  const firstId = doc.nodes[0].id;
  const first = page.locator(`[data-node-id="${firstId}"]`);
  await first.click({ position: { x: 20, y: 20 } });
  const width = doc.nodes[0].width;
  await drag(
    page,
    first.getByRole("button", {
      name: "调整 文字 bottom-right 尺寸",
      exact: true,
    }),
    64,
    32,
  );
  doc = await saved(page, id, doc.revision);
  expect(doc.nodes[0].width).toBeGreaterThan(width);
  await first.getByRole("button", { name: "重命名 文字", exact: true }).click();
  await first.getByLabel("节点名称", { exact: true }).fill("镜头想法");
  await first.getByLabel("节点名称", { exact: true }).press("Enter");
  doc = await saved(page, id, doc.revision);
  expect(doc.nodes[0].title).toBe("镜头想法");
  await drag(page, first.locator("header"), -460, -50);
  doc = await saved(page, id, doc.revision);
  await page.getByRole("button", { name: "文字", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  await page.getByRole("button", { name: "移动", exact: true }).click();
  const region = page.getByRole("region", { name: "画布编辑区", exact: true });
  const bounds = (await region.boundingBox())!;
  const viewport = { ...doc.viewport };
  await page.mouse.move(bounds.x + 100, bounds.y + 100);
  await page.mouse.down();
  await page.mouse.move(bounds.x + 180, bounds.y + 140, { steps: 8 });
  await page.mouse.up();
  doc = await saved(page, id, doc.revision);
  expect(doc.viewport.x).not.toBe(viewport.x);
  await page.getByRole("button", { name: "框选", exact: true }).click();
  const boxes = await Promise.all(
    doc.nodes.map((node) =>
      page.locator(`[data-node-id="${node.id}"]`).boundingBox(),
    ),
  );
  const left = Math.min(...boxes.map((box) => box!.x)) - 18;
  const top = Math.min(...boxes.map((box) => box!.y)) - 18;
  const right = Math.max(...boxes.map((box) => box!.x + box!.width)) + 18;
  const bottom = Math.max(...boxes.map((box) => box!.y + box!.height)) + 18;
  await page.mouse.move(left, top);
  await page.mouse.down();
  await page.mouse.move(right, bottom, { steps: 12 });
  await page.mouse.up();
  await expect(page.getByText("已选 2", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "分组", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  const group = doc.nodes.find((node) => node.node_type === "group")!;
  const groupElement = page.locator(`[data-node-id="${group.id}"]`);
  await groupElement.click({ position: { x: 12, y: 12 } });
  await groupElement
    .getByRole("button", { name: "折叠分组", exact: true })
    .click();
  doc = await saved(page, id, doc.revision);
  expect(doc.nodes.find((node) => node.id === group.id)!.config.collapsed).toBe(
    true,
  );
  await expect(first).toHaveCount(0);
  await groupElement
    .getByRole("button", { name: "展开分组", exact: true })
    .click();
  doc = await saved(page, id, doc.revision);
  expect(doc.nodes.find((node) => node.id === group.id)!.config.collapsed).toBe(
    false,
  );
  await expect(first).toBeVisible();
});

test("未保存文字阻止离开，继续编辑保留草稿", async ({ page }) => {
  const id = await create(page);
  let doc = await read(page, id);
  await page.getByRole("button", { name: "文字", exact: true }).click();
  doc = await saved(page, id, doc.revision);
  await page
    .locator(`[data-node-id="${doc.nodes[0].id}"]`)
    .click({ position: { x: 20, y: 20 } });
  await page.getByLabel("文字内容", { exact: true }).fill("未保存的创作草稿");
  await page.getByRole("link", { name: "Lanverse", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "放弃未保存修改？", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "继续编辑", exact: true }).click();
  await expect(page.getByLabel("文字内容", { exact: true })).toHaveValue(
    "未保存的创作草稿",
  );
  expect((await read(page, id)).nodes[0].config.text).toBe("");
});
