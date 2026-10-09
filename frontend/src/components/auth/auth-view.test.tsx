import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AuthView } from "./auth-view";
const mocks = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => mocks }));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
beforeEach(() => vi.clearAllMocks());
function fillRegistration() {
  fireEvent.change(screen.getByLabelText("显示名"), {
    target: { value: "Alice" },
  });
  fireEvent.change(screen.getByLabelText("登录名"), {
    target: { value: "alice" },
  });
  fireEvent.change(screen.getByLabelText("密码", { exact: true }), {
    target: { value: "Password12345" },
  });
  fireEvent.change(screen.getByLabelText("确认密码"), {
    target: { value: "Password12345" },
  });
}
it("注册失败保留输入且不进入工作台", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ code: "login_name_taken" }), {
        status: 422,
      }),
    ),
  );
  render(<AuthView mode="register" />);
  fillRegistration();
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toContain("已被使用"),
  );
  expect(mocks.push).not.toHaveBeenCalled();
  expect((screen.getByLabelText("登录名") as HTMLInputElement).value).toBe(
    "alice",
  );
});
it("注册等待真实回执，只有有效会话可以进入工作台", async () => {
  const session = {
    actor: {
      id: "11111111-1111-4111-8111-111111111111",
      login_name: "alice",
      display_name: "Alice",
      role: "creator",
      must_change_password: false,
      credential_revision: 1,
    },
    session_id: "22222222-2222-4222-8222-222222222222",
    last_active_at: new Date().toISOString(),
    absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
    persistent: false,
  };
  const request = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify(session), { status: 201 }));
  vi.stubGlobal("fetch", request);
  render(<AuthView mode="register" />);
  fillRegistration();
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  await waitFor(() => expect(mocks.push).toHaveBeenCalledWith("/"));
  expect(request.mock.calls[0][0]).toBe("/api/accounts/register");
});
it("无身份回执不伪造成功", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response("{}", { status: 201 })),
  );
  render(<AuthView mode="register" />);
  fillRegistration();
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toContain("回执"),
  );
  expect(mocks.push).not.toHaveBeenCalled();
});

it("受限改密使用会话版本，失败保留表单，成功后才继续", async () => {
  const session = {
    actor: {
      id: "11111111-1111-4111-8111-111111111111",
      login_name: "alice",
      display_name: "Alice",
      role: "creator",
      must_change_password: true,
      credential_revision: 7,
    },
    session_id: "22222222-2222-4222-8222-222222222222",
    last_active_at: new Date().toISOString(),
    absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
    persistent: false,
  };
  const updated = {
    ...session,
    actor: {
      ...session.actor,
      must_change_password: false,
      credential_revision: 8,
    },
    session_id: "33333333-3333-4333-8333-333333333333",
  };
  const request = vi
    .fn()
    .mockResolvedValueOnce(new Response(JSON.stringify(session)))
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ code: "current_password_invalid" }), {
        status: 422,
      }),
    )
    .mockResolvedValueOnce(new Response(JSON.stringify(updated)));
  vi.stubGlobal("fetch", request);
  render(<AuthView mode="reset-password" />);
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith("/api/session", expect.any(Object)),
  );
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "PreviousPassword123" },
  });
  fireEvent.change(screen.getByLabelText("新密码", { exact: true }), {
    target: { value: "NextPassword123" },
  });
  fireEvent.change(screen.getByLabelText("确认新密码"), {
    target: { value: "NextPassword123" },
  });
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "保存并继续" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存并继续" }));
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toContain("当前密码不正确"),
  );
  expect(mocks.push).not.toHaveBeenCalled();
  expect(JSON.parse(request.mock.calls[1][1].body)).toMatchObject({
    expected_credential_revision: 7,
  });
  expect(
    (screen.getByLabelText("新密码", { exact: true }) as HTMLInputElement)
      .value,
  ).toBe("NextPassword123");
  fireEvent.click(screen.getByRole("button", { name: "保存并继续" }));
  await waitFor(() => expect(mocks.push).toHaveBeenCalledWith("/"));
});
