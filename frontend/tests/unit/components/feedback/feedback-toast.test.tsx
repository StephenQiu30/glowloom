import { fireEvent, screen, waitFor, cleanup } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { render, findFeedback } from "../../../feedback";
import { FeedbackToast } from "@/components/feedback/feedback-toast";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
it("普通重渲染不重复弹出，重试使用最新回调", async () => {
  const error = vi.spyOn(toast, "error");
  const stale = vi.fn();
  const fresh = vi.fn();
  const view = render(
    <FeedbackToast
      message="读取失败"
      action={{ label: "重试", onClick: stale }}
    />,
  );
  await findFeedback();
  view.rerender(
    <FeedbackToast
      message="读取失败"
      action={{ label: "重试", onClick: fresh }}
    />,
  );
  expect(error).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(fresh).toHaveBeenCalledOnce();
  expect(stale).not.toHaveBeenCalled();
});
it("清除或离开视图会关闭对应提示，随后可再次提示相同错误", async () => {
  const error = vi.spyOn(toast, "error");
  const dismiss = vi.spyOn(toast, "dismiss");
  const view = render(<FeedbackToast message="上传失败" />);
  await findFeedback();
  const id = error.mock.calls[0][1]?.id;
  view.rerender(<FeedbackToast message="" />);
  expect(dismiss).toHaveBeenCalledWith(id);
  view.rerender(<FeedbackToast message="上传失败" />);
  expect(error).toHaveBeenCalledTimes(2);
  view.unmount();
  expect(dismiss).toHaveBeenLastCalledWith(id);
});
it("严格模式不堆叠同一条通知，错误可在关闭后重新触发", async () => {
  const view = render(
    <StrictMode>
      <FeedbackToast message="保存失败" />
    </StrictMode>,
  );
  await findFeedback();
  expect(
    document.querySelectorAll('[data-sonner-toast]:not([data-removed="true"])'),
  ).toHaveLength(1);
  view.rerender(
    <StrictMode>
      <FeedbackToast message="" />
    </StrictMode>,
  );
  await waitFor(() =>
    expect(
      toast.getToasts().filter((t) => "title" in t && t.title === "保存失败"),
    ).toHaveLength(0),
  );
});

it("操作提示不会被 Dialog 当作外部点击关闭表单", async () => {
  const { Dialog, DialogContent, DialogTitle, DialogDescription } =
    await import("@/components/ui/dialog");
  const close = vi.fn();
  const retry = vi.fn();
  render(
    <Dialog open onOpenChange={close}>
      <DialogContent>
        <DialogTitle>测试表单</DialogTitle>
        <DialogDescription>保留当前输入</DialogDescription>
        <FeedbackToast
          message="读取失败"
          action={{ label: "重试", onClick: retry }}
        />
      </DialogContent>
    </Dialog>,
  );
  await findFeedback();
  const button = screen.getByRole("button", { name: "重试" });
  fireEvent.pointerDown(button, { pointerType: "mouse" });
  fireEvent.click(button);
  expect(retry).toHaveBeenCalledOnce();
  expect(close).not.toHaveBeenCalledWith(false);
});
