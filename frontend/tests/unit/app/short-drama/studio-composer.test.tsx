import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StudioComposer } from "@/app/short-drama/studio-composer";

afterEach(cleanup);

it("500字计价单位不成为创作输入上限，添加推荐后保留已有长文本", () => {
  const onStart = vi.fn();
  render(<StudioComposer mode="create" disabled={false} onStart={onStart} />);
  const text = "甲".repeat(501);
  const editor = screen.getByLabelText("剧本创作要求");
  expect(editor.hasAttribute("maxlength")).toBe(false);
  fireEvent.change(editor, { target: { value: text } });
  fireEvent.click(screen.getByRole("button", { name: "剧本医生" }));
  expect((editor as HTMLTextAreaElement).value).toContain(text);
  fireEvent.click(screen.getByRole("button", { name: "立即生成" }));
  expect(onStart).toHaveBeenCalledWith({
    kind: "create",
    prompt: expect.stringContaining(text),
  });
});

it.each([
  ["create", "立即生成"],
  ["upload", "开始导入"],
  ["redraw", "开始转绘"],
] as const)("%s没有必要输入时不启动项目创建或生成", (mode, label) => {
  const onStart = vi.fn();
  render(<StudioComposer mode={mode} disabled={false} onStart={onStart} />);
  const start = screen.getByRole("button", { name: label });
  expect(start.hasAttribute("disabled")).toBe(true);
  fireEvent.click(start);
  expect(onStart).not.toHaveBeenCalled();
});

it("剧本文本忽略纯空白，有内容后才传入导入流程", () => {
  const onStart = vi.fn();
  render(<StudioComposer mode="upload" disabled={false} onStart={onStart} />);
  const editor = screen.getByLabelText("剧本文本");
  fireEvent.change(editor, { target: { value: " \n " } });
  expect(
    screen.getByRole("button", { name: "开始导入" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.change(editor, { target: { value: "第1集：雨夜" } });
  fireEvent.click(screen.getByRole("button", { name: "开始导入" }));
  expect(onStart).toHaveBeenCalledWith({
    kind: "upload",
    text: "第1集：雨夜",
    file: undefined,
  });
});

it("转绘拒绝错误格式，移除已选视频后重新禁止开始", () => {
  const onStart = vi.fn();
  const view = render(
    <StudioComposer mode="redraw" disabled={false} onStart={onStart} />,
  );
  const picker = view.container.querySelector('input[type="file"]')!;
  const start = screen.getByRole("button", { name: "开始转绘" });
  fireEvent.change(picker, {
    target: { files: [new File(["bad"], "notes.txt")] },
  });
  expect(screen.getByRole("alert").textContent).toContain("MP4 或 MOV");
  expect(start.hasAttribute("disabled")).toBe(true);
  fireEvent.change(picker, {
    target: {
      files: [new File(["video"], "source.mp4", { type: "video/mp4" })],
    },
  });
  expect(start.hasAttribute("disabled")).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "移除文件" }));
  expect(start.hasAttribute("disabled")).toBe(true);
  expect(onStart).not.toHaveBeenCalled();
});
