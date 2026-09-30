import { expect, test, type Page } from "@playwright/test";

// 合成 API 仅验证生产页面的控件、焦点与交互边界，不计真实 Go/数据库验收。
const projectId = "10000000-0000-4000-8000-000000000001";
const canvasId = "10000000-0000-4000-8000-000000000002";
const nodeId = "10000000-0000-4000-8000-000000000003";
async function openCanvas(page: Page, archived = false) {
  const writes: string[] = [];
  let mediaReads = 0;
  const document = {
    id: canvasId,
    project_id: projectId,
    name: "控件验证画布",
    revision: 1,
    scope: {},
    viewport: { x: 0, y: 0, zoom: 1 },
    nodes: [
      {
        id: nodeId,
        node_type: "text",
        node_action: "resource",
        title: "参考文字",
        config: { text: "仅验证界面" },
        x: 30,
        y: 60,
        width: 280,
        height: 180,
        z_index: 1,
      },
    ],
    edges: [],
  };
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() !== "GET") {
      writes.push(path);
      await route.fulfill({
        status: 500,
        json: { status: 500, code: "unexpected_write" },
      });
      return;
    }
    if (path === "/api/auth/me") {
      await route.fulfill({
        json: {
          user: {
            id: "10000000-0000-4000-8000-000000000010",
            org_id: "10000000-0000-4000-8000-000000000011",
            login_name: "synthetic-canvas-user",
            display_name: "界面验证",
            role: "producer",
            must_change_password: false,
          },
          must_change_password: false,
        },
      });
    } else if (path === "/api/projects") {
      await route.fulfill({
        json: {
          items: [
            {
              id: projectId,
              name: "控件验证项目",
              status: archived ? "archived" : "active",
              revision: 1,
            },
          ],
          next_cursor: null,
        },
      });
    } else if (path === `/api/projects/${projectId}/canvases`) {
      await route.fulfill({
        json: { items: [document], next_cursor: null },
      });
    } else if (path === `/api/canvases/${canvasId}`) {
      await route.fulfill({ json: document });
    } else if (path === `/api/projects/${projectId}/media`) {
      mediaReads += 1;
      await route.fulfill(
        mediaReads === 1
          ? {
              status: 503,
              contentType: "application/problem+json",
              json: {
                status: 503,
                code: "service_unavailable",
                title: "媒体库暂不可用",
              },
            }
          : { json: { items: [], next_cursor: null } },
      );
    } else {
      throw new Error(`Unexpected synthetic API read: ${path}`);
    }
  });
  await page.goto(`/projects/${projectId}/canvas?canvas=${canvasId}`);
  await expect(
    page.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
  return writes;
}

test("工具模式键盘导航不写画布，重复选择不会关闭当前模式", async ({ page }) => {
  const writes = await openCanvas(page);
  await page
    .locator(`[data-node-id="${nodeId}"]`)
    .click({ position: { x: 20, y: 20 } });
  const select = page.getByRole("radio", { name: "框选", exact: true });
  const move = page.getByRole("radio", { name: "移动", exact: true });
  await expect(select).toBeChecked();
  await select.focus();
  await page.keyboard.press("ArrowRight");
  await expect(move).toBeFocused();
  await page.keyboard.press("Space");
  await expect(move).toBeChecked();
  await move.click();
  await expect(move).toBeChecked();
  await expect(select).not.toBeChecked();
  expect(writes).toEqual([]);
});

test("工具提示、搜索和媒体失败恢复使用 Radix 焦点与官方状态组件", async ({
  page,
}, testInfo) => {
  const writes = await openCanvas(page);
  const search = page.getByRole("button", { name: "搜索", exact: true });
  await search.focus();
  await expect(page.getByRole("tooltip")).toContainText("搜索");
  await search.press("Enter");
  await page.getByLabel("搜索节点名称或文字").fill("不存在的节点");
  await expect(page.getByText("没有匹配的节点", { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(search).toBeFocused();
  const project = page.getByRole("combobox", { name: "项目", exact: true });
  await project.click();
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(project).toBeFocused();
  const media = page.getByRole("button", { name: "媒体库", exact: true });
  await media.click();
  const mediaFailure = page
    .getByRole("alert")
    .filter({ hasText: "媒体库读取失败" });
  await expect(mediaFailure).toBeVisible();
  await page.getByRole("button", { name: "重新读取", exact: true }).click();
  await expect(
    page.getByText("此项目暂无可用媒体", { exact: true }),
  ).toBeVisible();
  await expect(mediaFailure).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(media).toBeFocused();
  expect(writes).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("canvas-controls-desktop.png"),
    fullPage: true,
  });
});

test("390px 画布控件可使用，页面无横向溢出", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openCanvas(page);
  await page.getByRole("radio", { name: "移动", exact: true }).click();
  await expect(
    page.getByRole("radio", { name: "移动", exact: true }),
  ).toBeChecked();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("canvas-controls-mobile.png"),
    fullPage: true,
  });
});

test("归档项目的画布编辑与模式控件保持禁用", async ({ page }) => {
  const writes = await openCanvas(page, true);
  await expect(
    page.getByRole("status").filter({ hasText: "只读画布" }),
  ).toBeVisible();
  for (const name of ["框选", "移动"]) {
    await expect(page.getByRole("radio", { name, exact: true })).toBeDisabled();
  }
  for (const name of ["文字", "媒体库"]) {
    await expect(
      page.getByRole("button", { name, exact: true }),
    ).toBeDisabled();
  }
  expect(writes).toEqual([]);
});
