import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { Editor } from "@tiptap/core";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import { DocumentEditor } from "@/components/generation/document-editor";

afterEach(cleanup);
beforeAll(() => {
  Range.prototype.getClientRects = () => [] as unknown as DOMRectList;
  Range.prototype.getBoundingClientRect = () => new DOMRect();
});

it("正文格式经 Markdown 保存和重新打开保持一致，全屏不更换编辑器", async () => {
  let editor: Editor | null = null;
  let saved = "";
  const onChange = vi.fn((value: string) => {
    saved = value;
  });
  const view = render(
    <DocumentEditor
      id="document"
      text="正文"
      editable
      onChange={onChange}
      onReady={(value) => {
        editor = value;
      }}
    />,
  );
  await waitFor(() => expect(editor).toBeTruthy());
  act(() => {
    editor!.commands.selectAll();
    editor!
      .chain()
      .toggleHeading({ level: 2 })
      .toggleBold()
      .toggleUnderline()
      .setTextAlign("center")
      .run();
  });
  expect(saved).toContain("正文");
  const original = editor;
  fireEvent.click(screen.getByRole("button", { name: "全屏编辑文本" }));
  expect(screen.getByRole("dialog", { name: "文本编辑器" })).toBeTruthy();
  expect(editor).toBe(original);
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("textbox", { name: "文本内容" }),
    ),
  );
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  view.unmount();
  render(
    <DocumentEditor
      id="document"
      text={saved}
      editable
      onChange={vi.fn()}
      onReady={(value) => {
        editor = value;
      }}
    />,
  );
  await waitFor(() => expect(editor).not.toBe(original));
  const heading = editor!.getJSON().content![0];
  expect(heading.type).toBe("heading");
  expect(heading.attrs).toMatchObject({ level: 2, textAlign: "center" });
  expect(heading.content![0].marks?.map((mark) => mark.type)).toEqual(
    expect.arrayContaining(["bold", "underline"]),
  );
});

it("拒绝超长输入且保留可撤销的有效正文", async () => {
  let editor: Editor | null = null;
  const change = vi.fn();
  render(
    <DocumentEditor
      id="length-limit"
      text="保留正文"
      editable
      onChange={change}
      onReady={(value) => {
        editor = value;
      }}
    />,
  );
  await waitFor(() => expect(editor).toBeTruthy());
  act(() => {
    editor!.chain().selectAll().insertContent("长".repeat(10001)).run();
  });
  expect(screen.getByRole("alert").textContent).toContain("10000");
  expect(editor!.getText()).toBe("保留正文");
  expect(change).not.toHaveBeenCalled();
  act(() => {
    editor!.chain().selectAll().insertContent("有效修改").run();
  });
  expect(change).toHaveBeenLastCalledWith("有效修改");
  act(() => {
    editor!.commands.undo();
  });
  expect(editor!.getText()).toBe("保留正文");
});

it("只读正文不提供编辑工具，外部文字变化同步而不回写", async () => {
  const change = vi.fn();
  const view = render(
    <DocumentEditor
      id="document"
      text="原正文"
      editable={false}
      onChange={change}
    />,
  );
  await screen.findByText("原正文");
  expect(screen.queryByRole("toolbar")).toBeNull();
  view.rerender(
    <DocumentEditor
      id="document"
      text="远端正文"
      editable={false}
      onChange={change}
    />,
  );
  await screen.findByText("远端正文");
  expect(change).not.toHaveBeenCalled();
});
