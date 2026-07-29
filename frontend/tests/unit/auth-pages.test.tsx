import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
}));

import LoginPage from "@/app/login/page";
import RegisterPage from "@/app/register/page";
import { AppProviders } from "@/app/providers";

describe("authentication pages", () => {
  it("renders the login contract", () => {
    render(
      <AppProviders>
        <LoginPage />
      </AppProviders>,
    );

    expect(screen.getByRole("heading", { name: "登录 Lanverse" })).toBeInTheDocument();
    expect(screen.getByLabelText("邮箱")).toBeInTheDocument();
    expect(screen.getByLabelText("密码")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "登录" })).toBeInTheDocument();
  });

  it("renders the registration contract", () => {
    render(
      <AppProviders>
        <RegisterPage />
      </AppProviders>,
    );

    expect(screen.getByRole("heading", { name: "创建账号" })).toBeInTheDocument();
    expect(screen.getByLabelText("显示名称")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "注册并开始创作" })).toBeInTheDocument();
  });
});
