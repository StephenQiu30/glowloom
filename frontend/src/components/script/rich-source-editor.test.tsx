import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { RichSourceEditor } from "./rich-source-editor";
import { canonicalPlainText } from "./rich-document";
afterEach(cleanup);
const document = {
  type: "doc" as const,
  content: [
    {
      type: "paragraph" as const,
      content: [{ type: "text" as const, text: "保留😀" }],
    },
  ],
};
it("真实编辑模块客户端挂载、具名正文、只读状态禁用所有格式写入", async () => {
  const { rerender } = render(
    <RichSourceEditor
      initialDocument={document}
      disabled
      onChange={vi.fn()}
      onInvalid={vi.fn()}
    />,
  );
  const textbox = await screen.findByRole("textbox", { name: "来源正文" });
  expect(textbox.getAttribute("contenteditable")).toBe("false");
  expect(
    screen.getByRole("button", { name: "粗体" }).hasAttribute("disabled"),
  ).toBe(true);
  rerender(
    <RichSourceEditor
      initialDocument={document}
      disabled={false}
      onChange={vi.fn()}
      onInvalid={vi.fn()}
    />,
  );
  await waitFor(() =>
    expect(textbox.getAttribute("contenteditable")).toBe("true"),
  );
  expect(textbox.textContent).toBe("保留😀");
});
it("人工纯文本粘贴保留逐个换行与Unicode正文，不把换行扩大为段落分隔", async () => {
  const change = vi.fn();
  render(
    <RichSourceEditor
      initialDocument={{ type: "doc", content: [{ type: "paragraph" }] }}
      disabled={false}
      onChange={change}
      onInvalid={vi.fn()}
    />,
  );
  const textbox = await screen.findByRole("textbox", { name: "来源正文" });
  fireEvent.paste(textbox, {
    clipboardData: {
      getData: (type: string) =>
        type === "text/html"
          ? "<table><tr><td>原稿</td></tr></table>"
          : "甲😀\r\n\r\n乙\n",
    },
  });
  fireEvent.click(await screen.findByRole("button", { name: "仅粘贴纯文本" }));
  await waitFor(() => expect(change).toHaveBeenCalled());
  expect(canonicalPlainText(change.mock.lastCall![0])).toBe("甲😀\n\n乙\n");
});
it("剪贴板文件必须进入原文上传确认，正文不能无声丢弃附件", async () => {
  const invalid = vi.fn();
  const change = vi.fn();
  render(
    <RichSourceEditor
      initialDocument={document}
      disabled={false}
      onChange={change}
      onInvalid={invalid}
    />,
  );
  const textbox = await screen.findByRole("textbox", { name: "来源正文" });
  fireEvent.paste(textbox, {
    clipboardData: {
      files: [new File(["原文"], "原文.txt", { type: "text/plain" })],
      getData: () => "",
    },
  });
  expect(invalid).toHaveBeenCalledWith(
    "正文不直接接收文件。请使用项目原文导入入口并确认版权。",
  );
  expect(change).not.toHaveBeenCalled();
});
it("未知剪贴板格式不静默降级或改变正文，需明确选择纯文本", async () => {
  const change = vi.fn();
  render(
    <RichSourceEditor
      initialDocument={document}
      disabled={false}
      onChange={change}
      onInvalid={vi.fn()}
    />,
  );
  const textbox = await screen.findByRole("textbox", { name: "来源正文" });
  fireEvent.paste(textbox, {
    clipboardData: {
      getData: (type: string) =>
        type === "text/html" ? "<table><tr><td>表格</td></tr></table>" : "表格",
    },
  });
  expect(
    await screen.findByRole("dialog", { name: "确认剪贴板格式" }),
  ).toBeTruthy();
  expect(textbox.textContent).toBe("保留😀");
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "取消粘贴" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(change).not.toHaveBeenCalled();
});
