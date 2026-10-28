import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { UploadDropzone } from "@/components/media/upload-dropzone";
const file = new File(["fixture"], "参考.png", { type: "image/png" });
afterEach(cleanup);
function setup(disabled = false) {
  const onChoose = vi.fn(),
    onBrowse = vi.fn(),
    onError = vi.fn();
  render(
    <UploadDropzone
      file={file}
      disabled={disabled}
      onChoose={onChoose}
      onBrowse={onBrowse}
      onError={onError}
    />,
  );
  return {
    zone: screen.getByRole("group", { name: "文件上传区域" }),
    onChoose,
    onBrowse,
    onError,
  };
}
it("整块上传按钮打开文件选择，嵌套拖入移出保持反馈，放下只选中文件", () => {
  const { zone, onBrowse, onChoose } = setup();
  fireEvent.click(screen.getByRole("button", { name: "更换上传文件" }));
  expect(onBrowse).toHaveBeenCalledOnce();
  const dataTransfer = { types: ["Files"], files: [file] };
  fireEvent.dragEnter(zone, { dataTransfer });
  fireEvent.dragEnter(screen.getByText("松开即可添加"), { dataTransfer });
  fireEvent.dragLeave(screen.getByText("松开即可添加"));
  expect(screen.getByText("松开即可添加")).toBeTruthy();
  fireEvent.drop(zone, { dataTransfer });
  expect(onChoose).toHaveBeenCalledWith(file);
  expect(screen.queryByText("松开即可添加")).toBeNull();
});
it("移出上传区恢复原状态，不将文本拖拽当作文件", () => {
  const { zone } = setup();
  fireEvent.dragEnter(zone, { dataTransfer: { types: ["text/plain"] } });
  expect(screen.queryByText("松开即可添加")).toBeNull();
  fireEvent.dragEnter(zone, { dataTransfer: { types: ["Files"] } });
  fireEvent.dragLeave(zone);
  expect(screen.queryByText("松开即可添加")).toBeNull();
});
it("多文件拒绝，不修改调用方当前文件", () => {
  const { zone, onError, onChoose } = setup();
  fireEvent.drop(zone, { dataTransfer: { files: [file, file] } });
  expect(onError).toHaveBeenCalledWith("请每次上传一个文件。");
  expect(onChoose).not.toHaveBeenCalled();
  expect(screen.getByText("参考.png")).toBeTruthy();
});
it("上传中或结果待确认时拒绝拖入、替换和错误覆写", () => {
  const { zone, onError, onChoose, onBrowse } = setup(true);
  fireEvent.click(screen.getByRole("button", { name: "更换上传文件" }));
  fireEvent.dragEnter(zone, { dataTransfer: { types: ["Files"] } });
  fireEvent.drop(zone, { dataTransfer: { files: [file, file] } });
  expect(onChoose).not.toHaveBeenCalled();
  expect(onError).not.toHaveBeenCalled();
  expect(onBrowse).not.toHaveBeenCalled();
  expect(screen.getByText("文件已保留")).toBeTruthy();
});
