import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

test("固定样本、静止帧排除和测量导出保留真实证据边界", async ({ page }) => {
  await page.goto("/poc/canvas");
  await expect(
    page.getByRole("heading", { name: "无限画布性能验证" }),
  ).toBeVisible();
  await expect.poll(() => page.locator("video").count()).toBeGreaterThan(0);
  await page.getByRole("button", { name: "暂停视频预览" }).click();
  await expect
    .poll(() =>
      page
        .locator("video")
        .evaluateAll((videos) =>
          videos.every((video) => (video as HTMLVideoElement).paused),
        ),
    )
    .toBe(true);
  await page.getByRole("button", { name: "录制 60 秒交互" }).click();
  await page.getByRole("button", { name: "结束冒烟录制" }).click();
  await expect(page.getByRole("region", { name: "交互测量" })).toContainText(
    "缺少真实交互输入",
  );
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "导出测量 JSON" }).click();
  const download = await downloadPromise;
  const path = await download.path();
  expect(path).not.toBeNull();
  const record = JSON.parse(await readFile(path!, "utf8"));
  expect(record.schemaVersion).toBe(1);
  expect(record.meanFps).toBeNull();
  expect(record.trustedInputs).toBe(0);
  expect(record.device.viewport).toEqual([1440, 1000]);
  expect(record.scenario).toEqual({ nodes: 500, edges: 800 });
  await page.getByRole("button", { name: "2,000 节点 · 3,200 连线" }).click();
  await expect(page.locator("[data-scene]")).toHaveAttribute(
    "data-scene",
    "2000-3200",
  );
  await expect(page.getByRole("button", { name: "导出测量 JSON" })).toHaveCount(
    0,
  );
});

test("自选素材不上传，取消错误输入不替换已有样本", async ({ page }) => {
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  await page.goto("/poc/canvas");
  const input = page.getByLabel("自选差异素材");
  await input.setInputFiles({
    name: "wrong.pdf",
    mimeType: "application/pdf",
    buffer: Buffer.from("synthetic fixture"),
  });
  await expect(
    page.getByText("请选择 PNG、JPEG、WebP、AVIF 图片或 MP4、WebM 视频。"),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "恢复固定样本" }),
  ).toBeDisabled();
  await input.setInputFiles({
    name: "fixture.png",
    mimeType: "image/png",
    buffer: Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jwT8AAAAASUVORK5CYII=",
      "base64",
    ),
  });
  await expect(page.getByText(/已选择 1 张图 \/ 0 个视频/)).toBeVisible();
  await expect
    .poll(() => page.locator('img[src^="blob:"]').count())
    .toBeGreaterThan(0);
  expect(writes).toEqual([]);
  await page.getByRole("button", { name: "恢复固定样本" }).click();
  await expect(page.getByText(/固定样本作为对照/)).toBeVisible();
  await expect(page.locator('img[src^="blob:"]')).toHaveCount(0);
});

test("受控浏览器运动取得有限帧率，短时记录仍标为冒烟", async ({ page }) => {
  await page.goto("/poc/canvas");
  const pane = page.locator(".react-flow__pane");
  await expect(pane).toBeVisible();
  const box = (await pane.boundingBox())!;
  await page.getByRole("button", { name: "录制 60 秒交互" }).click();
  await page.mouse.move(box.x + 15, box.y + 20);
  await page.mouse.down();
  for (let index = 0; index < 20; index += 1) {
    await page.mouse.move(box.x + 15 + index * 2, box.y + 20 + index * 2, {
      steps: 3,
    });
  }
  await page.mouse.up();
  await page.getByRole("button", { name: "结束冒烟录制" }).click();
  await expect(page.getByRole("region", { name: "交互测量" })).toContainText(
    "短时冒烟记录",
  );
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "导出测量 JSON" }).click();
  const path = await (await downloadPromise).path();
  const record = JSON.parse(await readFile(path!, "utf8"));
  expect(record.trustedInputs).toBeGreaterThan(0);
  expect(record.frameIntervals).toBeGreaterThan(0);
  expect(record.meanFps).toBeGreaterThan(0);
  expect(record.activeMs).toBeLessThan(record.elapsedMs);
});

test("减少动态效果偏好禁用自动视频播放", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/poc/canvas");
  await expect(
    page.getByRole("button", { name: "播放视频预览" }),
  ).toBeDisabled();
  await expect
    .poll(() =>
      page
        .locator("video")
        .evaluateAll((videos) =>
          videos.every((video) => (video as HTMLVideoElement).paused),
        ),
    )
    .toBe(true);
});
