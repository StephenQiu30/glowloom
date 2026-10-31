import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { render, findFeedback } from "../../../feedback";
import { CanvasDeleteConfirmation } from "@/components/projects/canvas-delete-confirmation";

afterEach(cleanup);

it("默认聚焦取消，取消和Escape只关闭，确认才提交", async () => {
  const onClose = vi.fn(),
    onConfirm = vi.fn();
  render(
    <CanvasDeleteConfirmation
      description="是否确认删除所选画布？"
      locked={false}
      pending={false}
      retry={false}
      notice=""
      onClose={onClose}
      onConfirm={onConfirm}
      onCloseAutoFocus={() => {}}
    />,
  );
  const dialog = screen.getByRole("alertdialog", { name: "确认删除" });
  await waitFor(() =>
    expect(document.activeElement).toBe(
      within(dialog).getByRole("button", { name: "取消" }),
    ),
  );
  fireEvent.click(within(dialog).getByRole("button", { name: "取消" }));
  expect(onClose).toHaveBeenCalledTimes(1);
  expect(onConfirm).not.toHaveBeenCalled();
  fireEvent.keyDown(dialog, { key: "Escape" });
  expect(onClose).toHaveBeenCalledTimes(2);
  fireEvent.click(within(dialog).getByRole("button", { name: "删除" }));
  expect(onConfirm).toHaveBeenCalledTimes(1);
});

it("处理中或未知回执保持确认，不允许关闭；未知结果只重试原操作", async () => {
  const onClose = vi.fn(),
    onConfirm = vi.fn();
  const props = {
    description: "是否确认删除所选画布？",
    locked: true,
    pending: true,
    retry: true,
    notice: "",
    onClose,
    onConfirm,
    onCloseAutoFocus: () => {},
  };
  const view = render(<CanvasDeleteConfirmation {...props} />);
  const dialog = screen.getByRole("alertdialog");
  expect(within(dialog).getByRole("button", { name: "取消" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(
    within(dialog).queryByRole("button", { name: "关闭对话框" }),
  ).toBeNull();
  fireEvent.keyDown(dialog, { key: "Escape" });
  expect(onClose).not.toHaveBeenCalled();
  view.rerender(
    <CanvasDeleteConfirmation
      {...props}
      pending={false}
      notice="操作结果尚未确认，请重试原操作。"
    />,
  );
  await findFeedback("操作结果尚未确认");
  expect(within(dialog).queryByRole("alert")).toBeNull();
  fireEvent.click(within(dialog).getByRole("button", { name: "重试原操作" }));
  expect(onConfirm).toHaveBeenCalledTimes(1);
  expect(onClose).not.toHaveBeenCalled();
});
