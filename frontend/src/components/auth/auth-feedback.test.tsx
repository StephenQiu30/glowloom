import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { AuthView } from "./auth-view";
import { Toaster } from "@/components/ui/sonner";
import { request, RequestError } from "@/lib/request";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
afterEach(() => {
  toast.dismiss();
  cleanup();
});
it("登录失败通过全局提示展示，保留输入且不插入错误框", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("invalid_credentials", 401),
  );
  const { container } = render(
    <>
      <Toaster />
      <AuthView mode="login" embedded />
    </>,
  );
  fireEvent.change(screen.getByLabelText("邮箱"), {
    target: { value: "test@example.com" },
  });
  fireEvent.change(screen.getByLabelText("密码", { exact: true }), {
    target: { value: "password8" },
  });
  fireEvent.click(screen.getByRole("button", { name: "登录" }));
  await waitFor(() =>
    expect(document.querySelector("[data-sonner-toast]")).not.toBeNull(),
  );
  expect(container.querySelector('[data-slot="alert"]')).toBeNull();
  expect(screen.getByLabelText("邮箱")).toHaveProperty(
    "value",
    "test@example.com",
  );
  expect(screen.getByLabelText("密码", { exact: true })).toHaveProperty(
    "value",
    "password8",
  );
});

it("关闭校验提示后再次提交，仍提示并聚焦无效字段", async () => {
  vi.mocked(request).mockReset();
  render(
    <>
      <Toaster />
      <AuthView mode="login" embedded />
    </>,
  );
  fireEvent.click(screen.getByRole("button", { name: "登录" }));
  await screen.findByRole("button", { name: "关闭提示" });
  fireEvent.click(screen.getByRole("button", { name: "关闭提示" }));
  fireEvent.click(screen.getByRole("button", { name: "登录" }));
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByLabelText("邮箱")),
  );
  await screen.findByRole("button", { name: "关闭提示" });
  expect(request).not.toHaveBeenCalled();
  await waitFor(() =>
    expect(
      document.querySelectorAll(
        '[data-sonner-toast]:not([data-removed="true"])',
      ),
    ).toHaveLength(1),
  );
});
