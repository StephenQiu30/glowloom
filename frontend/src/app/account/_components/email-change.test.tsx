import { render } from "../../../../tests/feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { EmailChange } from "./email-change";
import { request } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  clear: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("next/navigation", () => ({ useRouter: () => mocks }));
vi.mock("@/components/auth/session", () => ({
  clearPrivateSession: mocks.clear,
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("两个邮箱必须分别验证，完成后清除秘密并要求重新登录", async () => {
  const id = "11111111-1111-4111-8111-111111111111";
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/me/email-change"
      ? { operation_id: id, retry_after_seconds: 60 }
      : undefined,
  );
  render(<EmailChange revision="7" onConflict={vi.fn()} />);
  fireEvent.change(screen.getByLabelText("新邮箱"), {
    target: { value: "new@example.com" },
  });
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "password" },
  });
  fireEvent.click(screen.getByText("验证密码并申请更换"));
  await screen.findByLabelText("原邮箱验证码");
  expect(screen.getByText("完成更换并退出登录")).toHaveProperty(
    "disabled",
    true,
  );
  for (const side of ["原邮箱验证码", "新邮箱验证码"]) {
    fireEvent.change(screen.getByLabelText(side), {
      target: { value: "123456" },
    });
    const form = screen.getByLabelText(side).closest("form")!;
    fireEvent.submit(form);
    await waitFor(() =>
      expect(screen.getByLabelText(side)).toHaveProperty("disabled", true),
    );
  }
  fireEvent.click(screen.getByText("完成更换并退出登录"));
  await waitFor(() =>
    expect(mocks.replace).toHaveBeenCalledWith("/login?email_changed=1"),
  );
  expect(request).toHaveBeenCalledWith(
    `/api/me/email-change/${id}/complete`,
    expect.objectContaining({ data: { expected_revision: "7" } }),
  );
});
