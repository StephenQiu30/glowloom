import { expect, test, type BrowserContext, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";

// 仅写入显式指定项目内、本测试新建的画布；不读取其他画布或调用供应商。
const projectId = process.env.LV_E2E_PROJECT_ID;
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
type CanvasDocument = {
  id: string;
  project_id: string;
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
  }[];
  edges: { id: string; source_node_id: string; target_node_id: string }[];
  viewport: { x: number; y: number; zoom: number };
};
type ClipboardGraph = {
  format: string;
  version: number;
  projectId: string;
  nodes: {
    id: string;
    type: string;
    parentId?: string;
    position: { x: number; y: number };
    config: { text?: string };
  }[];
  connections: { id: string; fromNodeId: string; toNodeId: string }[];
};

test.skip(!projectId, "需要 LV_E2E_PROJECT_ID 指向明确授权的隔离验收项目");
test.setTimeout(90_000);

async function read(page: Page, id: string): Promise<CanvasDocument> {
  const response = await page.request.get(`/api/canvases/${id}`);
  expect(response.status()).toBe(200);
  return response.json();
}
async function editor(page: Page) {
  await expect(
    page.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
}
async function create(page: Page) {
  await page
    .getByLabel("新画布名称", { exact: true })
    .fill(`迁移闭环-${crypto.randomUUID().slice(0, 8)}`);
  await page.getByRole("button", { name: "创建画布", exact: true }).click();
  await editor(page);
  const id = new URL(page.url()).searchParams.get("canvas");
  expect(id).toMatch(uuid);
  test.info().annotations.push({ type: "本测试画布", description: id! });
  return id!;
}
async function createViaAPI(page: Page) {
  const response = await page.request.post(
    `/api/projects/${projectId}/canvases`,
    {
      headers: {
        "Idempotency-Key": crypto.randomUUID(),
        Origin: new URL(page.url()).origin,
      },
      data: {
        name: `迁移剪贴板目标-${crypto.randomUUID().slice(0, 8)}`,
        scope: {},
      },
    },
  );
  expect(response.status()).toBe(201);
  const document: CanvasDocument = await response.json();
  test
    .info()
    .annotations.push({ type: "本测试画布", description: document.id });
  return document.id;
}
async function saved(page: Page, id: string, revision: number) {
  await expect
    .poll(async () => (await read(page, id)).revision)
    .toBe(revision + 1);
  const document = await read(page, id);
  await expect(
    page
      .getByRole("status")
      .filter({ hasText: `已保存 · 修订 ${document.revision}` }),
  ).toBeVisible();
  return document;
}
async function commit(
  page: Page,
  id: string,
  document: CanvasDocument,
  action: () => Promise<unknown>,
  types?: string[],
) {
  const [response] = await Promise.all([
    page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === `/api/canvases/${id}/commands` &&
        response.request().method() === "POST",
      { timeout: 15_000 },
    ),
    action(),
  ]);
  expect(response.status()).toBe(200);
  const body = response.request().postDataJSON() as {
    expected_revision: number;
    commands: { type: string }[];
  };
  expect(body.expected_revision).toBe(document.revision);
  expect(response.request().headers()["idempotency-key"]).toBeTruthy();
  if (types)
    expect(body.commands.map((command) => command.type)).toEqual(types);
  return saved(page, id, document.revision);
}
async function blank(page: Page, x = 100, y = 120) {
  const viewport = page.locator("[data-canvas-viewport]");
  const box = await viewport.boundingBox();
  expect(box).not.toBeNull();
  return { x: box!.x + x, y: box!.y + y };
}
async function createByDoubleClick(
  page: Page,
  id: string,
  document: CanvasDocument,
  x: number,
  y: number,
) {
  const point = await blank(page, x, y);
  await page.mouse.dblclick(point.x, point.y);
  await expect(
    page.getByRole("menuitem", { name: "文字节点", exact: true }),
  ).toBeVisible();
  return commit(
    page,
    id,
    document,
    () => page.getByRole("menuitem", { name: "文字节点", exact: true }).click(),
    ["AddNodes"],
  );
}
async function connectToBlank(
  page: Page,
  id: string,
  document: CanvasDocument,
) {
  const source = page.locator(
    `[data-node-id="${document.nodes[0].id}"] [data-node-handle="source"]`,
  );
  const box = await source.boundingBox();
  expect(box).not.toBeNull();
  const point = await blank(page, 600, 340);
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await page.mouse.down();
  await page.mouse.move(point.x, point.y, { steps: 8 });
  await page.mouse.up();
  await expect(
    page.getByRole("menuitem", { name: "文字节点", exact: true }),
  ).toBeVisible();
  return commit(
    page,
    id,
    document,
    () => page.getByRole("menuitem", { name: "文字节点", exact: true }).click(),
    ["AddNodes", "Connect"],
  );
}
async function selectAll(page: Page) {
  await page.locator("[data-canvas-viewport]").focus();
  await page.keyboard.press("ControlOrMeta+a");
}
async function contextMenu(page: Page) {
  const point = await blank(page, 24, 480);
  expect(
    await page.evaluate(
      ({ x, y }) =>
        Boolean(
          globalThis.document
            .elementFromPoint(x, y)
            ?.closest("[data-canvas-viewport]"),
        ) &&
        !globalThis.document
          .elementFromPoint(x, y)
          ?.closest("[data-canvas-no-zoom]"),
      point,
    ),
  ).toBe(true);
  await page.mouse.click(point.x, point.y, { button: "right" });
  await expect(
    page.getByRole("menuitem", { name: "添加文字", exact: true }),
  ).toBeVisible();
}
async function systemClipboard(page: Page, value: string) {
  // 先写入本测试内容，后续读取只检查本轮创建的数据。
  await page.evaluate((text) => navigator.clipboard.writeText(text), value);
}
async function clipboard(page: Page) {
  return page.evaluate(() => navigator.clipboard.readText());
}
async function clipboardPermissions(context: BrowserContext) {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
}
async function persisted(page: Page, id: string, document: CanvasDocument) {
  await page.reload();
  await editor(page);
  expect(await read(page, id)).toEqual(document);
  const documentPath = test
    .info()
    .outputPath(`正式画布-${id}-修订${document.revision}.json`);
  await writeFile(documentPath, JSON.stringify(document, null, 2));
  await test.info().attach(`正式画布-${id}.json`, {
    path: documentPath,
    contentType: "application/json",
  });
  const screenshotPath = test
    .info()
    .outputPath(`刷新后画布-${id}-修订${document.revision}.png`);
  await page
    .getByRole("region", { name: "无限画布编辑器", exact: true })
    .screenshot({ path: screenshotPath });
  await test.info().attach(`刷新后画布-${id}.png`, {
    path: screenshotPath,
    contentType: "image/png",
  });
}

test.beforeEach(async ({ page, context }) => {
  await clipboardPermissions(context);
  await page.goto(`/canvas?project=${projectId}`);
});

test("双击与空白连线创建、右键分组和布局原子保存后可刷新", async ({ page }) => {
  const id = await create(page);
  let document = await read(page, id);
  document = await createByDoubleClick(page, id, document, 120, 140);
  const sourceId = document.nodes[0].id;
  document = await connectToBlank(page, id, document);
  expect(document.nodes).toHaveLength(2);
  expect(document.edges).toHaveLength(1);
  expect(document.edges[0].source_node_id).toBe(sourceId);
  expect(
    document.nodes.some((node) => node.id === document.edges[0].target_node_id),
  ).toBe(true);

  await selectAll(page);
  await contextMenu(page);
  await page.getByRole("menuitem", { name: "排列节点", exact: true }).hover();
  await expect(
    page.getByRole("menuitem", { name: "水平排列", exact: true }),
  ).toBeVisible();
  document = await commit(page, id, document, () =>
    page.getByRole("menuitem", { name: "水平排列", exact: true }).click(),
  );
  await expect(page.getByRole("menu")).toHaveCount(0);
  expect(new Set(document.nodes.map((node) => node.y)).size).toBe(1);
  expect(new Set(document.nodes.map((node) => node.x)).size).toBe(2);

  await selectAll(page);
  await contextMenu(page);
  document = await commit(page, id, document, () =>
    page.getByRole("menuitem", { name: "组成分组", exact: true }).click(),
  );
  const group = document.nodes.find((node) => node.node_type === "group")!;
  expect(group).toBeTruthy();
  expect(
    document.nodes.filter((node) => node.parent_id === group.id),
  ).toHaveLength(2);
  expect(document.edges).toHaveLength(1);
  await persisted(page, id, document);
  await expect(page.locator(`[data-node-id="${group.id}"]`)).toBeVisible();
});

test("背景、小地图、焦点与视图快捷键保留原生 Tab 和弹窗操作", async ({
  page,
}) => {
  const id = await create(page);
  let document = await read(page, id);
  document = await createByDoubleClick(page, id, document, 120, 130);
  document = await connectToBlank(page, id, document);
  const viewport = page.locator("[data-canvas-viewport]");

  await page.getByRole("combobox", { name: "画布背景", exact: true }).click();
  await page.getByRole("option", { name: "纯色", exact: true }).click();
  await expect(page.locator("[data-canvas-grid-layer]")).toHaveCount(0);
  await page.getByRole("combobox", { name: "画布背景", exact: true }).click();
  await page.getByRole("option", { name: "网格", exact: true }).click();
  await expect(page.locator("[data-canvas-grid-layer]")).toBeVisible();
  await expect(page.locator("[data-canvas-grid-layer]")).toHaveCSS(
    "background-image",
    /linear-gradient/,
  );

  await page.getByRole("button", { name: "小地图", exact: true }).click();
  await expect(
    page.getByRole("img", { name: "画布缩略地图", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "小地图", exact: true }).click();
  const map = page.getByRole("img", { name: "画布缩略地图", exact: true });
  await expect(map).toBeVisible();
  document = await commit(
    page,
    id,
    document,
    () => map.click({ position: { x: 35, y: 40 } }),
    ["SetViewport"],
  );
  document = await commit(
    page,
    id,
    document,
    () => page.getByRole("button", { name: "缩小画布", exact: true }).click(),
    ["SetViewport"],
  );
  expect(document.viewport.zoom).toBeLessThan(1);

  await viewport.focus();
  document = await commit(
    page,
    id,
    document,
    () => page.keyboard.press("ControlOrMeta+1"),
    ["SetViewport"],
  );
  expect(document.viewport.zoom).toBe(1);
  await viewport.focus();
  document = await commit(
    page,
    id,
    document,
    () => page.keyboard.press("ControlOrMeta+2"),
    ["SetViewport"],
  );
  await page
    .locator(`[data-node-id="${document.nodes[0].id}"]`)
    .click({ position: { x: 20, y: 20 } });
  document = await commit(
    page,
    id,
    document,
    () => page.keyboard.press("ControlOrMeta+3"),
    ["SetViewport"],
  );

  await viewport.focus();
  await page.keyboard.press("Tab");
  await expect(
    page.getByRole("complementary", { name: "节点属性", exact: true }),
  ).toHaveCount(0);
  await viewport.focus();
  await page.keyboard.press("Tab");
  await expect(
    page.getByRole("complementary", { name: "节点属性", exact: true }),
  ).toBeVisible();
  const helpButton = page.getByRole("button", {
    name: "快捷键帮助",
    exact: true,
  });
  await helpButton.focus();
  await page.keyboard.press("Tab");
  await expect(
    page.getByRole("complementary", { name: "节点属性", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "进入焦点模式", exact: true }),
  ).toBeFocused();

  await viewport.focus();
  await page.keyboard.press("?");
  const dialog = page.getByRole("dialog", { name: "画布快捷键", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText("100% 缩放", { exact: true })).toBeVisible();
  await page.keyboard.press("ControlOrMeta+1");
  expect((await read(page, id)).revision).toBe(document.revision);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await persisted(page, id, document);
});

test("原生文字复制和真实系统剪贴板跨标签及刷新保持闭集与新 UUID", async ({
  page,
  context,
}) => {
  const id = await create(page);
  let document = await read(page, id);
  document = await createByDoubleClick(page, id, document, 120, 130);
  const text = `本轮原生文字复制-${crypto.randomUUID()}`;
  const textNode = page.locator(`[data-node-id="${document.nodes[0].id}"]`);
  await textNode.click({ position: { x: 20, y: 20 } });
  await page.getByLabel("文字内容", { exact: true }).fill(text);
  document = await commit(
    page,
    id,
    document,
    () => page.getByRole("button", { name: "保存文字", exact: true }).click(),
    ["UpdateNodeConfig"],
  );
  await systemClipboard(page, "本测试复制前的占位文字");
  await page.getByLabel("文字内容", { exact: true }).selectText();
  await page.keyboard.press("ControlOrMeta+c");
  await expect.poll(() => clipboard(page)).toBe(text);
  await page.locator("[data-canvas-viewport]").focus();
  await page
    .getByRole("complementary", { name: "节点属性", exact: true })
    .getByText("文字", { exact: true })
    .evaluate((element) => {
      const range = globalThis.document.createRange();
      range.selectNodeContents(element);
      window.getSelection()?.removeAllRanges();
      window.getSelection()?.addRange(range);
    });
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe(
    "文字",
  );
  await page.keyboard.press("ControlOrMeta+c");
  await expect.poll(() => clipboard(page)).toBe("文字");
  expect((await read(page, id)).revision).toBe(document.revision);
  await page.evaluate(() => window.getSelection()?.removeAllRanges());

  document = await connectToBlank(page, id, document);
  await selectAll(page);
  await contextMenu(page);
  document = await commit(page, id, document, () =>
    page.getByRole("menuitem", { name: "组成分组", exact: true }).click(),
  );
  const group = document.nodes.find((node) => node.node_type === "group")!;
  await page
    .locator(`[data-node-id="${group.id}"]`)
    .click({ position: { x: 12, y: 12 } });
  await page.getByRole("button", { name: "复制", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "已复制 3 个节点到系统剪贴板" }),
  ).toBeVisible();
  const wire = JSON.parse(await clipboard(page)) as ClipboardGraph;
  expect(wire).toMatchObject({
    format: "lanverse.canvas",
    version: 1,
    projectId,
  });
  expect(wire.nodes).toHaveLength(3);
  expect(wire.connections).toHaveLength(1);
  expect(Object.keys(wire).sort()).toEqual([
    "connections",
    "format",
    "nodes",
    "projectId",
    "version",
  ]);
  expect(
    wire.nodes.every(
      (node) =>
        !["media", "metadata", "url", "blob"].some((key) => key in node),
    ),
  ).toBe(true);

  const targetId = await createViaAPI(page);
  const targetPage = await context.newPage();
  await targetPage.goto(`/canvas?project=${projectId}&canvas=${targetId}`);
  await editor(targetPage);
  let target = await read(targetPage, targetId);
  target = await commit(
    targetPage,
    targetId,
    target,
    () => targetPage.getByRole("button", { name: "粘贴", exact: true }).click(),
    ["AddNodes", "Connect"],
  );
  expect(target.nodes).toHaveLength(3);
  expect(target.edges).toHaveLength(1);
  const originalIds = new Set([
    ...wire.nodes.map((node) => node.id),
    ...wire.connections.map((edge) => edge.id),
  ]);
  expect(
    [
      ...target.nodes.map((node) => node.id),
      ...target.edges.map((edge) => edge.id),
    ].every((value) => uuid.test(value) && !originalIds.has(value)),
  ).toBe(true);
  const targetGroup = target.nodes.find((node) => node.node_type === "group")!;
  expect(
    target.nodes.filter((node) => node.parent_id === targetGroup.id),
  ).toHaveLength(2);
  const targetIds = new Set(target.nodes.map((node) => node.id));
  expect(
    target.edges.every(
      (edge) =>
        targetIds.has(edge.source_node_id) &&
        targetIds.has(edge.target_node_id),
    ),
  ).toBe(true);
  await persisted(targetPage, targetId, target);
  target = await commit(
    targetPage,
    targetId,
    target,
    () => targetPage.getByRole("button", { name: "粘贴", exact: true }).click(),
    ["AddNodes", "Connect"],
  );
  expect(target.nodes).toHaveLength(6);
  expect(new Set(target.nodes.map((node) => node.id)).size).toBe(6);
  expect(target.edges).toHaveLength(2);
  await persisted(targetPage, targetId, target);
  await targetPage.close();
});

test("跨项目、越界载荷及剪贴板权限失败显式拒绝，原生文字可保存", async ({
  page,
  context,
}) => {
  const id = await create(page);
  let document = await read(page, id);
  document = await createByDoubleClick(page, id, document, 120, 130);
  await page
    .locator(`[data-node-id="${document.nodes[0].id}"]`)
    .click({ position: { x: 20, y: 20 } });
  await page.getByRole("button", { name: "复制", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "已复制 1 个节点到系统剪贴板" }),
  ).toBeVisible();
  const wire = JSON.parse(await clipboard(page)) as ClipboardGraph;

  await systemClipboard(
    page,
    JSON.stringify({ ...wire, projectId: crypto.randomUUID() }),
  );
  await page.getByRole("button", { name: "粘贴", exact: true }).click();
  await expect(
    page
      .getByRole("status")
      .filter({ hasText: "剪贴板画布属于其他项目，不能粘贴到当前项目" }),
  ).toBeVisible();
  expect(await read(page, id)).toEqual(document);
  await systemClipboard(
    page,
    JSON.stringify({
      ...wire,
      nodes: wire.nodes.map((node) => ({
        ...node,
        url: "https://invalid.example/test-private-preview",
      })),
    }),
  );
  await page.getByRole("button", { name: "粘贴", exact: true }).click();
  await expect(
    page
      .getByRole("status")
      .filter({ hasText: "剪贴板画布数据无效或超出容量限制" }),
  ).toBeVisible();
  expect(await read(page, id)).toEqual(document);

  const plainText = `原生剪贴板文字-${crypto.randomUUID()}`;
  await systemClipboard(page, plainText);
  document = await commit(
    page,
    id,
    document,
    () => page.getByRole("button", { name: "粘贴", exact: true }).click(),
    ["AddNodes"],
  );
  expect(document.nodes.some((node) => node.config.text === plainText)).toBe(
    true,
  );
  await context.clearPermissions();
  await page.getByRole("button", { name: "粘贴", exact: true }).click();
  await expect(
    page
      .getByRole("status")
      .filter({ hasText: "读取系统剪贴板失败，请允许剪贴板权限后重试" }),
  ).toBeVisible();
  expect(await read(page, id)).toEqual(document);
  await clipboardPermissions(context);
  await persisted(page, id, document);
});
