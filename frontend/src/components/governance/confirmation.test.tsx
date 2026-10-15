import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Confirmation } from "./confirmation";
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
