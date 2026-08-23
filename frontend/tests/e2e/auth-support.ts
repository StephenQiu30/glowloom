import { expect, type Page } from "@playwright/test";

const E2E_REGISTRATION_CODE = "123456";

type RegistrationValues = {
  displayName: string;
  email: string;
  password?: string;
};

export async function registerUser(
  page: Page,
  {
    displayName,
    email,
    password = "playwright-secure-password",
  }: RegistrationValues,
) {
  await page.goto("/register");
  await page.getByLabel("邮箱").fill(email);
  await page.getByRole("button", { name: "发送验证码" }).click();
  await expect(page.getByText(`验证码已经发送至 ${email}`)).toBeVisible();

  await page.getByLabel("验证码").fill(E2E_REGISTRATION_CODE);
  await page.getByRole("button", { name: "确认验证码" }).click();
  await expect(page.getByText("邮箱已验证")).toBeVisible();

  await page.getByLabel("显示名称").fill(displayName);
  await page.getByLabel("密码").fill(password);
  await page.getByLabel("我已阅读并同意服务协议与隐私政策").check();
  await page.getByRole("button", { name: "注册并开始创作" }).click();
  await expect(page).toHaveURL(/\/projects$/);
}
