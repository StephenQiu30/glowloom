import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  login: vi.fn(),
  register: vi.fn(),
}));

const routerReplace = vi.hoisted(() => vi.fn());

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: routerReplace }),
}));

vi.mock("@/api/identity", async () => {
  const actual = await vi.importActual<typeof import("@/api/identity")>(
    "@/api/identity",
  );
  return {
    ...actual,
    loginApiV1AuthLoginPost: apiMocks.login,
    registerApiV1AuthRegisterPost: apiMocks.register,
  };
});

import LoginPage from "@/app/login/page";
import RegisterPage from "@/app/register/page";
import { AppProviders } from "@/app/providers";

const authResponse: API.AuthResponse = {
  user: {
    id: "019fb1f2-5709-7e1b-bf17-b221f4fb8e09",
    email: "creator@example.com",
    display_name: "漫剧创作者",
    avatar_url: null,
  },
  workspace: {
    id: "019fb1f2-570a-78f7-aa45-54408769c2d4",
    name: "漫剧创作者的空间",
    status: "active",
    role: "owner",
    revision: 1,
  },
  access_token: "real-api-access-token",
  token_type: "bearer",
  expires_in: 1800,
};

describe("authentication pages", () => {
  beforeEach(() => {
    sessionStorage.clear();
    routerReplace.mockReset();
    apiMocks.login.mockReset();
    apiMocks.register.mockReset();
    apiMocks.login.mockResolvedValue({ data: authResponse });
    apiMocks.register.mockResolvedValue({ data: authResponse });
  });

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
    expect(screen.getByRole("img", { name: "她从画中来项目画面" })).toHaveAttribute(
      "src",
      "/assets/lanverse-studio/painting-girl-cover.png",
    );
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

  it("logs in through the generated API and persists the returned access token", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <LoginPage />
      </AppProviders>,
    );

    await user.clear(screen.getByLabelText("邮箱"));
    await user.type(screen.getByLabelText("邮箱"), "creator@example.com");
    await user.clear(screen.getByLabelText("密码"));
    await user.type(screen.getByLabelText("密码"), "secure-password-123");
    await user.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() => expect(apiMocks.login).toHaveBeenCalledTimes(1));
    expect(apiMocks.login).toHaveBeenCalledWith({
      email: "creator@example.com",
      password: "secure-password-123",
    });
    expect(sessionStorage.getItem("lanverse.access-token")).toBe(
      "real-api-access-token",
    );
    expect(routerReplace).toHaveBeenCalledWith("/projects");
  });

  it("registers through the generated API with the typed profile", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <RegisterPage />
      </AppProviders>,
    );

    await user.type(screen.getByLabelText("显示名称"), "漫剧创作者");
    await user.clear(screen.getByLabelText("邮箱"));
    await user.type(screen.getByLabelText("邮箱"), "creator@example.com");
    await user.clear(screen.getByLabelText("密码"));
    await user.type(screen.getByLabelText("密码"), "secure-password-123");
    await user.click(screen.getByRole("button", { name: "注册并开始创作" }));

    await waitFor(() => expect(apiMocks.register).toHaveBeenCalledTimes(1));
    expect(apiMocks.register).toHaveBeenCalledWith({
      display_name: "漫剧创作者",
      email: "creator@example.com",
      password: "secure-password-123",
    });
    expect(sessionStorage.getItem("lanverse.access-token")).toBe(
      "real-api-access-token",
    );
    expect(routerReplace).toHaveBeenCalledWith("/projects");
  });
});
