import { expect, type Page } from "@playwright/test";

export const ONE_PIXEL_PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
  "base64",
);

export function testToneWav(): Buffer {
  const sampleRate = 8_000;
  const sampleCount = sampleRate / 4;
  const dataSize = sampleCount * 2;
  const wav = Buffer.alloc(44 + dataSize);
  wav.write("RIFF", 0);
  wav.writeUInt32LE(36 + dataSize, 4);
  wav.write("WAVE", 8);
  wav.write("fmt ", 12);
  wav.writeUInt32LE(16, 16);
  wav.writeUInt16LE(1, 20);
  wav.writeUInt16LE(1, 22);
  wav.writeUInt32LE(sampleRate, 24);
  wav.writeUInt32LE(sampleRate * 2, 28);
  wav.writeUInt16LE(2, 32);
  wav.writeUInt16LE(16, 34);
  wav.write("data", 36);
  wav.writeUInt32LE(dataSize, 40);
  for (let index = 0; index < sampleCount; index += 1) {
    const sample = Math.round(
      6_553 * Math.sin((2 * Math.PI * 440 * index) / sampleRate),
    );
    wav.writeInt16LE(sample, 44 + index * 2);
  }
  return wav;
}

export type AssetFixture = {
  consentReference: string;
  kind: "character" | "location" | "voice";
  mediaName: string;
  name: string;
  tabName: "角色" | "场景" | "声音";
};

export async function uploadAndWait(
  page: Page,
  file: { buffer: Buffer; mimeType: string; name: string },
) {
  await page.getByLabel("选择媒体文件").setInputFiles(file);
  await page.getByRole("button", { name: "上传并开始探测" }).click();
  await expect(page.getByRole("status")).toContainText("媒体探测任务已创建");
  await expect
    .poll(
      async () => {
        await page.reload();
        return await page
          .locator("article")
          .filter({ hasText: file.name })
          .textContent({ timeout: 2_000 })
          .catch(() => "");
      },
      { timeout: 20_000 },
    )
    .toContain("可用");
}

export async function fillAssetSpec(page: Page, fixture: AssetFixture) {
  if (fixture.kind === "character") {
    await page.getByLabel("身份定位").fill("女主角");
    await page.getByLabel("外观描述").fill("乌发高髻，青灰色长衫，右眼有泪痣");
    await page.getByLabel("年龄观感").fill("二十五岁");
    await page.getByLabel("性格特征（逗号分隔）").fill("清冷，克制");
    return;
  }
  if (fixture.kind === "location") {
    await page.getByLabel("空间描述").fill("长安旧城的青石雨巷，两侧为木构店铺");
    await page.getByLabel("时间与天气").fill("深夜，小雨");
    await page.getByLabel("视觉元素（逗号分隔）").fill("青石路，灯笼，雨幕");
    await page.getByLabel("光线描述").fill("暖色灯笼与冷色月光形成对比");
    return;
  }
  await page.getByLabel("声音来源").selectOption("synthetic_recording");
  await page.getByLabel("语言").fill("zh-CN");
  await page.getByLabel("表演特征（逗号分隔）").fill("克制，清晰");
  await page.getByLabel("允许用途（逗号分隔）").fill("角色对白，内部预览");
}

export async function createReadyAsset(page: Page, fixture: AssetFixture) {
  await page.goto("/studio");
  await expect(page.getByRole("heading", { name: "资产库" })).toBeVisible();
  await page.getByRole("button", { name: "新建资产" }).click();
  const createDialog = page.getByRole("dialog", { name: "新建资产身份" });
  await createDialog.getByLabel("资产类型").selectOption(fixture.kind);
  await createDialog.getByLabel("资产名称").fill(fixture.name);
  await createDialog.getByLabel("别名（逗号分隔）").fill(`${fixture.name}别名`);
  await createDialog.getByLabel("标签（逗号分隔）").fill("端到端验收，第一季");
  await createDialog.getByRole("button", { name: "创建资产" }).click();
  await expect(page.getByRole("status")).toContainText("资产身份已创建");

  await page.getByRole("button", { name: "添加新版本" }).click();
  await fillAssetSpec(page, fixture);
  await page.getByLabel("参考媒体").selectOption({
    label: `${fixture.mediaName} · v1 · ready`,
  });
  await page.getByLabel("提示词描述").fill("保持当前已确认的视觉或表演特征。");
  await page.getByRole("button", { name: "保存版本" }).click();
  await expect(page.getByRole("status")).toContainText("准备度为已阻断");
  await expect(page.getByText("缺少覆盖当前用途的有效授权")).toBeVisible();

  await page.getByRole("link", { name: "前往授权治理" }).click();
  await expect(page.getByRole("heading", { name: "登记新授权" })).toBeVisible();
  await page.getByLabel("权利主体引用").fill(fixture.consentReference);
  await page.getByLabel("登记说明").fill("固定版本用于 AI 漫剧生成与平台预览");
  await page.getByRole("button", { name: "登记授权" }).click();
  await expect(page.getByRole("status")).toContainText("授权已登记");

  await page.goto("/studio");
  await page.getByRole("tab", { name: fixture.tabName }).click();
  await expect(
    page.getByRole("button", { name: `选择资产 ${fixture.name}` }),
  ).toBeVisible();
  await expect(page.getByText("媒体、字段与授权范围均满足当前用途")).toBeVisible();
}
