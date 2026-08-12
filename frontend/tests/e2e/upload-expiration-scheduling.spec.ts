import { createHash } from "node:crypto";

import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

const backendPort = process.env.LANVERSE_E2E_BACKEND_PORT ?? "8687";
const backendBaseUrl = `http://127.0.0.1:${backendPort}`;

test("上传到期计划可暂停、立即触发、恢复并从任务事实收敛", async ({ page }) => {
  test.setTimeout(90_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `上传调度-${unique}`;

  await registerUser(page, {
    displayName: "调度验收创作者",
    email: `schedule-${unique}@example.com`,
  });

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: /创建(?:第一集|单集)/ }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("上传清理验收集");
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: "进入上传清理验收集" }).click();
  await expect(page).toHaveURL(/\/studio\/[^/]+\/script$/);
  const episodeId = new URL(page.url()).pathname.split("/")[2];

  const accessToken = await page.evaluate(() =>
    window.sessionStorage.getItem("lanverse.access-token"),
  );
  expect(accessToken).toBeTruthy();
  const headers = { authorization: `Bearer ${accessToken}` };
  const me = await page.request.get(`${backendBaseUrl}/api/v1/me`, { headers });
  expect(me.status()).toBe(200);
  const workspaceId = (await me.json()).data.workspace.id as string;
  const content = Buffer.from("unconfirmed-upload-for-schedule-e2e");
  const initialized = await page.request.post(
    `${backendBaseUrl}/api/v1/media/uploads`,
    {
      headers,
      data: {
        workspace_id: workspaceId,
        kind: "image",
        filename: `temporary-${unique}.png`,
        size_bytes: content.length,
        mime_type: "image/png",
        sha256: createHash("sha256").update(content).digest("hex"),
        idempotency_key: `schedule-e2e-${unique}`,
      },
    },
  );
  expect(initialized.status()).toBe(201);

  await page.goto(`/studio/${episodeId}/tasks`);
  const schedule = page.locator("article").filter({ hasText: "单次上传到期清理" });
  const cleanupSchedule = page
    .locator("article")
    .filter({ hasText: "周期过期上传补偿" });
  await expect(schedule.getByText("运行中")).toBeVisible();
  await expect(cleanupSchedule).toBeVisible();

  await cleanupSchedule.getByRole("button", { name: "配置周期" }).click();
  const configurationDialog = page.getByRole("dialog", {
    name: "配置补偿清理周期",
  });
  await configurationDialog
    .getByRole("combobox", { name: "计划类型" })
    .click();
  await page.getByRole("option", { name: "Cron + IANA 时区" }).click();
  await configurationDialog.getByLabel("数字五段 Cron").fill("0 3 * * *");
  await configurationDialog.getByLabel("IANA 时区").fill("Asia/Shanghai");
  await configurationDialog
    .getByRole("combobox", { name: "停机补偿策略" })
    .click();
  await page.getByRole("option", { name: "有界逐次补执" }).click();
  await configurationDialog.getByLabel("最多补执次数").fill("2");
  await configurationDialog
    .getByRole("button", { name: "保存计划配置" })
    .click();
  await expect(page.getByRole("status")).toContainText("计划配置已保存");
  await expect(cleanupSchedule).toContainText("Cron 0 3 * * *");
  await expect(cleanupSchedule).toContainText("Asia/Shanghai");
  await expect(cleanupSchedule).toContainText("最多 2 次");

  await schedule.getByRole("button", { name: "暂停" }).click();
  await expect(page.getByRole("status")).toContainText("清理计划已暂停");
  await expect(schedule.getByText("已暂停")).toBeVisible();

  await schedule.getByRole("button", { name: "立即触发" }).click();
  await expect(page.getByRole("status")).toContainText("清理任务已创建");
  await expect(page.getByText("上传临时文件清理").first()).toBeVisible({
    timeout: 15_000,
  });

  await schedule.getByRole("button", { name: "恢复并执行" }).click();
  const resumeDialog = page.getByRole("dialog", { name: "恢复调度计划" });
  await resumeDialog.getByRole("button", { name: "确认恢复" }).click();
  await expect(page.getByRole("status")).toContainText("清理计划已恢复");
  await expect(schedule.getByText("已完成")).toBeVisible({ timeout: 20_000 });
  await expect(schedule.getByRole("button", { name: "立即触发" })).toHaveCount(0);
});
