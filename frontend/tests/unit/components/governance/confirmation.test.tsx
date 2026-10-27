import { render } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Confirmation } from "@/components/governance/confirmation";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("冲突重读一次且不重试业务动作，关闭清除密码", async () => {
  vi.mocked(request).mockResolvedValue(undefined);
  const run = vi
    .fn()
    .mockRejectedValue(new RequestError("revision_conflict", 409));
  const done = vi.fn();
  const close = vi.fn();
  const action = {
    title: "停用成员",
    effect: "停止团队访问",
    reason: true,
    run,
  };
  const { rerender } = render(
    <Confirmation action={action} onDone={done} onClose={close} />,
  );
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "secret" },
  });
  fireEvent.change(screen.getByLabelText("操作原因"), {
    target: { value: "测试治理原因" },
  });
  fireEvent.click(screen.getByText("确认操作"));
  await screen.findByRole("alert");
  expect(run).toHaveBeenCalledTimes(1);
  expect(done).toHaveBeenCalledTimes(1);
  expect(close).not.toHaveBeenCalled();
  expect(screen.getByLabelText("当前密码")).toHaveProperty("value", "");
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "new-secret" },
  });
  expect(screen.getByText("确认操作")).toHaveProperty("disabled", true);
  rerender(
    <Confirmation key="closed" action={null} onDone={done} onClose={close} />,
  );
  expect(screen.queryByLabelText("当前密码")).toBeNull();
});
it("错误密码不执行业务动作", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("invalid_credentials", 401),
  );
  const run = vi.fn();
  render(
    <Confirmation
      action={{ title: "禁用", effect: "停止访问", run }}
      onDone={vi.fn()}
      onClose={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "wrong" },
  });
  fireEvent.click(screen.getByText("确认操作"));
  await screen.findByRole("alert");
  await waitFor(() => expect(run).not.toHaveBeenCalled());
});
it("简短删除确认使用警示对话框，取消优先聚焦，失败保留确认入口", async () => {
  const run = vi.fn().mockRejectedValue(new Error("failed"));
  const close = vi.fn();
  render(
    <Confirmation
      action={{
        title: "删除测试用户？",
        effect: "删除后无法登录，作品保留。此操作不可恢复。",
        confirmLabel: "删除",
        destructive: true,
        reason: false,
        password: false,
        run,
      }}
      onDone={vi.fn()}
      onClose={close}
    />,
  );
  expect(
    screen.getByRole("alertdialog", { name: "删除测试用户？" }),
  ).toBeTruthy();
  expect(screen.queryByLabelText("当前密码")).toBeNull();
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "取消" }),
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  await screen.findByRole("alert");
  expect(close).not.toHaveBeenCalled();
  expect(run).toHaveBeenCalledTimes(1);
});
