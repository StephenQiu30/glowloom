import { render, findFeedback } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AuthView } from "@/components/auth/auth-view";
import { AuthDialog } from "@/components/auth/auth-dialog";
import { requireSession } from "@/components/auth/contracts";
import {
  configureSessionRenewal,
  disposeSessionRenewal,
} from "@/components/auth/session-renewal";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
const session = {
  actor: {
    id: "11111111-1111-4111-8111-111111111111",
    email: "alice@example.com",
    display_name: "Alice",
    platform_role: "creator",
    status: "active",
    revision: "1",
    capabilities: ["profile:write"],
  },
  session_id: "22222222-2222-4222-8222-222222222222",
  credential_epoch: "1",
  csrf_token: "session-bound-proof",
  must_change_password: false,
  session_protocol: "v1",
  upgrade_available: false,
  server_time: new Date().toISOString(),
  idle_expires_at: new Date(Date.now() + 43200000).toISOString(),
  persistent: false,
  last_active_at: new Date().toISOString(),
  absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
};
const policy = {
  registration_enabled: true,
  terms_version: "2026-10-15.1",
  available_terms_version: "2026-10-15.1",
  closed_reason: "",
  terms_url: "/terms",
  privacy_url: "/privacy",
};

function mockRegistration(
  completion: unknown = { account_id: session.actor.id, next: "login" },
) {
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/auth-policy"
      ? policy
      : url === "/api/registrations"
        ? {
            operation_id: "33333333-3333-4333-8333-333333333333",
            retry_after_seconds: 60,
          }
        : url.endsWith("/verify")
          ? { outcome: "verified", registration_grant: "single-use-grant" }
          : completion,
  );
}
async function requestCode() {
  await screen.findByText("协议版本 2026-10-15.1");
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "alice@example.com" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发送验证码" }));
  await screen.findByLabelText("邮箱验证码");
}
async function verifyCode() {
  await requestCode();
  fireEvent.change(screen.getByLabelText("邮箱验证码"), {
    target: { value: "123456" },
  });
  fireEvent.click(screen.getByRole("button", { name: "验证邮箱" }));
  await screen.findByLabelText("显示名");
}
function fillAccount(name = "Alice") {
  fireEvent.change(screen.getByLabelText("显示名"), {
    target: { value: name },
  });
  fireEvent.change(screen.getByLabelText("新密码", { exact: true }), {
    target: { value: "abC789xy" },
  });
  fireEvent.change(screen.getByLabelText("确认新密码"), {
    target: { value: "abC789xy" },
  });
  fireEvent.click(screen.getByRole("checkbox"));
}
it("更换邮箱放弃旧流程且不自动发信，重新提交绑定新邮箱", async () => {
  mockRegistration();
  render(<AuthView mode="register" />);
  await requestCode();
  fireEvent.click(screen.getByRole("button", { name: "更换邮箱" }));
  expect(screen.queryByLabelText("邮箱验证码")).toBeNull();
  expect(document.activeElement).toBe(screen.getByLabelText("邮箱"));
  expect(request).toHaveBeenCalledTimes(2);
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "bob@example.com" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发送验证码" }));
  await waitFor(() => expect(request).toHaveBeenCalledTimes(3));
  expect(request).toHaveBeenLastCalledWith(
    "/api/registrations",
    expect.objectContaining({
      data: expect.objectContaining({ email: "bob@example.com" }),
    }),
  );
});
it("注册步骤可读，格式错误验证码不消耗服务端尝试", async () => {
  mockRegistration();
  render(<AuthView mode="register" />);
  await requestCode();
  expect(
    screen
      .getByRole("list", { name: "注册步骤" })
      .querySelector('[aria-current="step"]')?.textContent,
  ).toContain("验证邮箱");
  fireEvent.change(screen.getByLabelText("邮箱验证码"), {
    target: { value: "123" },
  });
  fireEvent.click(screen.getByRole("button", { name: "验证邮箱" }));
  expect(await findFeedback("请输入 6 位数字验证码。")).toBeTruthy();
  expect(document.activeElement).toBe(screen.getByLabelText("邮箱验证码"));
  expect(request).toHaveBeenCalledTimes(2);
});
it.each(["", "字".repeat(33), "bad\u0001name"])(
  "显示名无效时定位字段且不提交建号 %j",
  async (name) => {
    mockRegistration();
    render(<AuthView mode="register" />);
    await verifyCode();
    fillAccount(name);
    fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByLabelText("显示名")),
    );
    expect(request).toHaveBeenCalledTimes(3);
  },
);
it("异常建号回执不显示成功、不建立会话", async () => {
  mockRegistration({});
  render(<AuthView mode="register" />);
  await verifyCode();
  fillAccount();
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  expect(await findFeedback("暂时无法确认结果，请重试。")).toBeTruthy();
  expect(screen.queryByText("账号已创建")).toBeNull();
  expect(router.replace).not.toHaveBeenCalled();
});
it("弹窗注册成功明确转登录，保留邮箱并清空密码，不自动认证或跳转", async () => {
  mockRegistration();
  const authenticated = vi.fn();
  render(
    <AuthDialog
      open
      initialMode="register"
      onOpenChange={vi.fn()}
      onAuthenticated={authenticated}
    />,
  );
  await verifyCode();
  fillAccount();
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  expect(await screen.findByText("账号已创建")).toBeTruthy();
  expect(authenticated).not.toHaveBeenCalled();
  expect(router.replace).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "前往登录" }));
  expect(screen.getByLabelText("邮箱")).toHaveProperty(
    "value",
    "alice@example.com",
  );
  expect(screen.getByLabelText("密码", { exact: true })).toHaveProperty(
    "value",
    "",
  );
});
it("弹窗切换仅保留邮箱，关闭重开清空表单", async () => {
  mockRegistration();
  const props = { onOpenChange: vi.fn(), onAuthenticated: vi.fn() };
  const view = render(<AuthDialog open {...props} />);
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "alice@example.com" },
  });
  fireEvent.change(screen.getByLabelText("密码", { exact: true }), {
    target: { value: "private-password" },
  });
  fireEvent.mouseDown(screen.getByRole("tab", { name: "注册" }), {
    button: 0,
    ctrlKey: false,
  });
  await screen.findByText("协议版本 2026-10-15.1");
  expect(screen.getByLabelText("邮箱")).toHaveProperty(
    "value",
    "alice@example.com",
  );
  fireEvent.mouseDown(screen.getByRole("tab", { name: "登录" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(screen.getByLabelText("密码", { exact: true })).toHaveProperty(
    "value",
    "",
  );
  view.rerender(<AuthDialog open={false} {...props} />);
  view.rerender(<AuthDialog open {...props} />);
  expect(screen.getByLabelText("邮箱")).toHaveProperty("value", "");
});
afterEach(() => {
  cleanup();
  disposeSessionRenewal();
});
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.clearAllMocks();
  vi.mocked(request).mockReset();
});
function login() {
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "alice@example.com" },
  });
  fireEvent.change(screen.getByLabelText("密码", { exact: true }), {
    target: { value: "LongPassword12345" },
  });
  fireEvent.click(screen.getByRole("button", { name: "登录" }));
}
it("email login uses real generated contract and restricted session redirects", async () => {
  vi.mocked(request).mockResolvedValue(session);
  const authenticated = vi.fn();
  const view = render(
    <AuthView mode="login" embedded onAuthenticated={authenticated} />,
  );
  login();
  await waitFor(() => expect(authenticated).toHaveBeenCalledWith(session));
  expect(request).toHaveBeenCalledWith(
    "/api/sessions",
    expect.objectContaining({
      data: {
        email: "alice@example.com",
        password: "LongPassword12345",
        persistent: false,
      },
    }),
  );
  view.unmount();
  vi.mocked(request).mockResolvedValue({
    ...session,
    must_change_password: true,
  });
  render(<AuthView mode="login" embedded onAuthenticated={authenticated} />);
  login();
  await waitFor(() =>
    expect(authenticated).toHaveBeenLastCalledWith(
      expect.objectContaining({ must_change_password: true }),
    ),
  );
  expect(router.push).not.toHaveBeenCalledWith("/reset-password");
});
it("cancelled modal ignores late authentication receipt", async () => {
  let resolve!: (value: unknown) => void;
  vi.mocked(request).mockReturnValue(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const authenticated = vi.fn();
  const view = render(
    <AuthView mode="login" embedded onAuthenticated={authenticated} />,
  );
  login();
  view.unmount();
  resolve(session);
  await Promise.resolve();
  expect(authenticated).not.toHaveBeenCalled();
});
it("closed server policy disables registration and provides readable agreements", async () => {
  vi.mocked(request).mockResolvedValue({
    ...policy,
    registration_enabled: false,
    closed_reason: "registration_closed",
  });
  render(<AuthView mode="register" />);
  expect(await screen.findByText(/注册暂未开放/)).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "发送验证码" }).hasAttribute("disabled"),
  ).toBe(true);
});
it("邮件不可用时说明恢复方式并阻止发码", async () => {
  vi.mocked(request).mockResolvedValue({
    ...policy,
    registration_enabled: false,
    closed_reason: "mail_unavailable",
  });
  render(<AuthView mode="register" />);
  expect(await screen.findByText(/验证邮件服务暂不可用/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "发送验证码" })).toHaveProperty(
    "disabled",
    true,
  );
});
it("registration verifies email then requires explicit agreement and server version", async () => {
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/auth-policy"
      ? policy
      : url === "/api/registrations"
        ? {
            operation_id: "33333333-3333-4333-8333-333333333333",
            retry_after_seconds: 60,
          }
        : url.endsWith("/verify")
          ? { outcome: "verified", registration_grant: "single-use-grant" }
          : { account_id: session.actor.id, next: "login" },
  );
  render(<AuthView mode="register" />);
  await screen.findByText("协议版本 2026-10-15.1");
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "alice@example.com" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发送验证码" }));
  fireEvent.change(await screen.findByLabelText("邮箱验证码"), {
    target: { value: "123456" },
  });
  fireEvent.click(screen.getByRole("button", { name: "验证邮箱" }));
  fireEvent.change(await screen.findByLabelText("显示名"), {
    target: { value: "Alice" },
  });
  fireEvent.change(screen.getByLabelText("新密码", { exact: true }), {
    target: { value: "abC789xy" },
  });
  fireEvent.change(screen.getByLabelText("确认新密码"), {
    target: { value: "abC789xy" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  expect(await findFeedback("请先阅读并同意协议。")).toHaveProperty(
    "textContent",
    "请先阅读并同意协议。",
  );
  expect(request).toHaveBeenCalledTimes(3);
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  expect(await screen.findByText("账号已创建")).toBeTruthy();
  expect(router.replace).not.toHaveBeenCalled();
  expect(request).toHaveBeenLastCalledWith(
    expect.stringContaining("/complete"),
    expect.objectContaining({
      data: expect.objectContaining({
        terms_version: "2026-10-15.1",
        terms_accepted: true,
      }),
    }),
  );
});
it("recovery acknowledges queued request without claiming mail delivery", async () => {
  vi.mocked(request).mockResolvedValue({
    operation_id: "33333333-3333-4333-8333-333333333333",
    retry_after_seconds: 60,
  });
  render(<AuthView mode="recover" />);
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "alice@example.com" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发送恢复链接" }));
  expect(await findFeedback()).toHaveProperty(
    "textContent",
    expect.stringContaining("不代表邮件已送达"),
  );
});
it("network failure preserves visible input", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("temporarily_unavailable", 503),
  );
  render(<AuthView mode="login" />);
  login();
  expect(await findFeedback()).toBeTruthy();
  expect(screen.getByLabelText("邮箱")).toHaveProperty(
    "value",
    "alice@example.com",
  );
});

it("invalid receipt never reports authentication success", async () => {
  vi.mocked(request).mockResolvedValue({});
  const onAuthenticated = vi.fn();
  render(<AuthView mode="login" onAuthenticated={onAuthenticated} />);
  login();
  expect(await findFeedback()).toBeTruthy();
  expect(onAuthenticated).not.toHaveBeenCalled();
  expect(router.push).not.toHaveBeenCalled();
});
it("empty login focuses first invalid field without sending a request", async () => {
  render(<AuthView mode="login" />);
  fireEvent.click(screen.getByRole("button", { name: "登录" }));
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByLabelText("邮箱")),
  );
  expect(request).not.toHaveBeenCalled();
  expect(screen.getByLabelText("邮箱").getAttribute("aria-invalid")).toBe(
    "true",
  );
  expect(screen.getByLabelText("邮箱").getAttribute("aria-describedby")).toBe(
    document.querySelector("form .sr-only")?.id,
  );
});
it("optional password change uses session epoch and retains input after failure", async () => {
  let failed = true;
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/session")
      return { ...session, must_change_password: true, credential_epoch: "42" };
    if (failed) throw new RequestError("revision_conflict", 409);
    return session;
  });
  render(<AuthView mode="reset-password" />);
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "保存新密码" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "current password" },
  });
  fireEvent.change(screen.getByLabelText("新密码", { exact: true }), {
    target: { value: "another long password" },
  });
  fireEvent.change(screen.getByLabelText("确认新密码"), {
    target: { value: "another long password" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存新密码" }));
  await findFeedback();
  expect(screen.getByLabelText("新密码", { exact: true })).toHaveProperty(
    "value",
    "another long password",
  );
  expect(router.push).not.toHaveBeenCalled();
  failed = false;
  fireEvent.click(screen.getByRole("button", { name: "保存新密码" }));
  await waitFor(() => expect(router.push).toHaveBeenCalledWith("/"));
  expect(request).toHaveBeenLastCalledWith(
    "/api/me/password",
    expect.objectContaining({
      data: expect.objectContaining({ expected_credential_epoch: "42" }),
    }),
  );
});
it("restricted password page explains the requirement and clears an inline validation error while editing", async () => {
  vi.mocked(request).mockResolvedValue({
    ...session,
    must_change_password: true,
  });
  const { container } = render(<AuthView mode="reset-password" />);
  await screen.findByText("验证当前密码，为账号设置新的登录密码。");
  expect(screen.getByRole("link", { name: "返回账号设置" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "使用其他账号登录" })).toBeTruthy();
  expect(screen.getByText("使用 8～128 个字符，支持空格和中文。")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "current password" },
  });
  fireEvent.change(screen.getByLabelText("新密码", { exact: true }), {
    target: { value: "abcdef7" },
  });
  fireEvent.change(screen.getByLabelText("确认新密码"), {
    target: { value: "abcdef7" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存新密码" }));
  const fieldError = await findFeedback();
  expect(fieldError.closest("[data-sonner-toast]")).not.toBeNull();
  expect(container.querySelector('[data-slot="alert"]')).toBeNull();
  expect(
    screen
      .getByLabelText("新密码", { exact: true })
      .getAttribute("aria-describedby"),
  ).toBe(document.querySelector("form .sr-only")?.id);
  expect(request).toHaveBeenCalledTimes(1);
  fireEvent.change(screen.getByLabelText("新密码", { exact: true }), {
    target: { value: "a new long password" },
  });
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByLabelText("当前密码")).toHaveProperty(
    "value",
    "current password",
  );
  expect(
    screen
      .getByLabelText("新密码", { exact: true })
      .getAttribute("aria-invalid"),
  ).toBe("false");
});

it("ordinary password update can return to account settings without a mandatory warning", async () => {
  vi.mocked(request).mockResolvedValue(session);
  render(<AuthView mode="reset-password" />);
  const back = await screen.findByRole("link", { name: "返回账号设置" });
  expect(back.getAttribute("href")).toBe("/account");
  expect(
    screen.queryByText("当前账号需要先更新密码，完成后即可继续创作。"),
  ).toBeNull();
  expect(request).toHaveBeenCalledTimes(1);
});
it("pending login submits once", async () => {
  vi.mocked(request).mockReturnValue(new Promise(() => {}));
  render(<AuthView mode="login" />);
  login();
  fireEvent.submit(screen.getByRole("form", { name: "登录织光表单" }));
  expect(request).toHaveBeenCalledTimes(1);
});
it("SMTP recovery fragment submits proof without querying a logged-in session", async () => {
  window.history.replaceState(null, "", "/reset-password#token=one-time-proof");
  vi.mocked(request).mockResolvedValue(undefined);
  render(<AuthView mode="reset-password" />);
  await screen.findByLabelText("新密码", { exact: true });
  expect(window.location.hash).toBe("");
  expect(request).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("新密码", { exact: true }), {
    target: { value: "abC789xy" },
  });
  fireEvent.change(screen.getByLabelText("确认新密码"), {
    target: { value: "abC789xy" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存新密码" }));
  await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/login"));
  expect(request).toHaveBeenCalledWith(
    "/api/password-resets/complete",
    expect.objectContaining({
      data: expect.objectContaining({ token: "one-time-proof" }),
    }),
  );
});

it("StrictMode recovery retains the captured proof after clearing the fragment", async () => {
  window.history.replaceState(null, "", "/reset-password#token=strict-proof");
  vi.mocked(request).mockResolvedValue(undefined);
  const { StrictMode } = await import("react");
  render(
    <StrictMode>
      <AuthView mode="reset-password" />
    </StrictMode>,
  );
  await screen.findByRole("button", { name: "保存新密码" });
  expect(request).not.toHaveBeenCalled();
  expect(window.location.hash).toBe("");
});
it("failed registration policy ends loading and offers retry", async () => {
  vi.mocked(request)
    .mockRejectedValueOnce(new RequestError("temporarily_unavailable", 503))
    .mockResolvedValueOnce(policy);
  render(<AuthView mode="register" />);
  await findFeedback();
  expect(screen.queryByText("正在读取注册策略…")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重新读取注册策略" }));
  expect(await screen.findByText("协议版本 2026-10-15.1")).toBeTruthy();
});
it("malformed accepted mail receipt never shows queued success", async () => {
  vi.mocked(request).mockResolvedValue({
    operation_id: "wrong",
    retry_after_seconds: -1,
  });
  render(<AuthView mode="recover" />);
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "alice@example.com" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发送恢复链接" }));
  await findFeedback();
  expect(screen.queryByText(/请求已受理，不代表邮件已送达/)).toBeNull();
});

it.each(["abC789xy", "织光画布创作世界", "x".repeat(128)])(
  "valid new password reaches the existing password update endpoint (%s)",
  async (password) => {
    vi.mocked(request).mockResolvedValue(session);
    render(<AuthView mode="reset-password" />);
    await screen.findByRole("link", { name: "返回账号设置" });
    fireEvent.change(screen.getByLabelText("当前密码"), {
      target: { value: "existing password" },
    });
    for (const label of ["新密码", "确认新密码"]) {
      fireEvent.change(screen.getByLabelText(label, { exact: true }), {
        target: { value: password },
      });
    }
    fireEvent.click(screen.getByRole("button", { name: "保存新密码" }));
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/me/password",
        expect.objectContaining({
          data: expect.objectContaining({
            new_password: password,
            confirm_password: password,
          }),
        }),
      ),
    );
  },
);

it("recovery keeps an invalid email error visible", async () => {
  render(<AuthView mode="recover" />);
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "invalid" },
  });
  fireEvent.submit(screen.getByRole("form"));
  expect(await findFeedback()).toHaveProperty(
    "textContent",
    "请输入有效的邮箱地址。",
  );
  expect(request).not.toHaveBeenCalled();
});

it("optional password page can switch accounts only after revoking the current session", async () => {
  configureSessionRenewal(requireSession(session), () => {});
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session"
      ? { ...session, must_change_password: true }
      : undefined,
  );
  render(<AuthView mode="reset-password" />);
  const button = await screen.findByRole("button", {
    name: "使用其他账号登录",
  });
  fireEvent.click(button);
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      `/api/sessions/${session.session_id}`,
      expect.objectContaining({ method: "DELETE" }),
    ),
  );
  await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/login"));
});

it("failed account switching stays on the form and preserves input", async () => {
  configureSessionRenewal(requireSession(session), () => {});
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/session")
      return { ...session, must_change_password: true };
    throw new RequestError("temporarily_unavailable");
  });
  render(<AuthView mode="reset-password" />);
  const button = await screen.findByRole("button", {
    name: "使用其他账号登录",
  });
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "fixture current" },
  });
  fireEvent.click(button);
  expect(await findFeedback()).toHaveProperty(
    "textContent",
    "服务暂时不可用，请稍后重试。",
  );
  expect(screen.getByLabelText("当前密码")).toHaveProperty(
    "value",
    "fixture current",
  );
  expect(router.replace).not.toHaveBeenCalled();
});

it("受邀注册在已生效条款下保留邀请连续性，初次验证码请求绑定令牌", async () => {
  const { readInvitationFragment, clearInvitation } =
    await import("@/components/auth/invitation-continuity");
  const invitation = "a".repeat(43);
  window.history.replaceState(null, "", `/invitations#token=${invitation}`);
  readInvitationFragment();
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/auth-policy"
      ? {
          ...policy,
          registration_enabled: false,
          terms_version: "2026-10-15.1",
        }
      : {
          operation_id: "11111111-1111-4111-8111-111111111111",
          retry_after_seconds: 60,
        },
  );
  render(<AuthView mode="register" />);
  const email = await screen.findByLabelText("邮箱");
  fireEvent.change(email, { target: { value: "invited@example.com" } });
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "发送验证码" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "发送验证码" }));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/registrations",
      expect.objectContaining({
        data: { email: "invited@example.com", invitation_token: invitation },
      }),
    ),
  );
  clearInvitation();
});

it.each(["origin_forbidden", "origin_mismatch"])(
  "origin error %s renders a trusted canonical link and preserves login input",
  async (code) => {
    vi.stubGlobal("location", new URL("http://localhost:3200/login"));
    vi.mocked(request).mockRejectedValue(
      new RequestError(code, 403, {
        canonical_origin: "http://127.0.0.1:3200",
      }),
    );
    try {
      render(<AuthView mode="login" embedded />);
      login();
      expect(
        (
          await screen.findByRole("link", { name: "使用可信网站地址打开" })
        ).getAttribute("href"),
      ).toBe("http://127.0.0.1:3200/login");
      expect(screen.getByLabelText("邮箱")).toHaveProperty(
        "value",
        "alice@example.com",
      );
      expect(screen.getByLabelText("密码", { exact: true })).toHaveProperty(
        "value",
        "LongPassword12345",
      );
    } finally {
      vi.unstubAllGlobals();
    }
  },
);
