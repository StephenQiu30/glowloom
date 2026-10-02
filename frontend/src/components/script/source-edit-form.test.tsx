import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SourceEditForm } from "./source-edit-form";
vi.mock("next/dynamic", () => ({
  default:
    () =>
    ({
      disabled,
      onChange,
    }: {
      disabled: boolean;
      onChange: (value: unknown) => void;
    }) => (
      <button
        type="button"
        disabled={disabled}
        onClick={() =>
          onChange({
            type: "doc",
            content: [
              {
                type: "paragraph",
                content: [
                  { type: "text", text: "正文😀", marks: [{ type: "bold" }] },
                ],
              },
            ],
          })
        }
      >
        测试仅格式变化
      </button>
    ),
}));
beforeEach(() =>
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  ),
);
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const source = {
  id: "11111111-1111-4111-8111-111111111111",
  source_lineage_id: "22222222-2222-4222-8222-222222222222",
  source_revision: 2,
  source_kind: "chapter" as const,
  title: "一",
  status: "ready" as const,
  origin: "file" as const,
  position: 0,
  char_count: 3,
  content_hash: "a".repeat(64),
  rich_sha256: "b".repeat(64),
  document: {
    type: "doc" as const,
    content: [
      {
        type: "paragraph" as const,
        content: [{ type: "text" as const, text: "正文😀" }],
      },
    ],
  },
  plain_text: "正文😀",
  source_span: {
    source_id: "11111111-1111-4111-8111-111111111111",
    source_lineage_id: "22222222-2222-4222-8222-222222222222",
    position: 0,
    start: 0,
    end: 3,
  },
  provenance: {
    external_position: 9,
    encoding: "utf-8",
    mapping: [{ part: "text", span_start: 0, span_end: 3 }],
    warnings: [
      { code: "embedded_media_omitted" as const, count: 2, paragraph: 0 },
    ],
  },
};
it("格式变化单独作为完整新rich写入，冻结CAS并仅继承外部标签", async () => {
  const submit = vi.fn();
  const dirty = vi.fn();
  render(
    <SourceEditForm
      source={source}
      base={{ expected_revision: 4 }}
      locked={false}
      readOnly={false}
      onSubmit={submit}
      onDirty={dirty}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "测试仅格式变化" }));
  expect(
    screen.getByText("内嵌媒体未进入正文 · 2 处 · 原件第 1 段"),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("checkbox", { name: "已获授权使用此正文" }));
  fireEvent.click(screen.getByRole("button", { name: "保存来源" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    expected_revision: 4,
    title: "一",
    provenance: { external_position: 9 },
    document: {
      content: [{ content: [{ text: "正文😀", marks: [{ type: "bold" }] }] }],
    },
  });
  expect(submit.mock.calls[0][0].provenance).not.toHaveProperty("encoding");
  expect(submit.mock.calls[0][0].provenance).not.toHaveProperty("warnings");
  expect(dirty).toHaveBeenCalledWith(true);
});
it("标题超预算保留原草稿且不给DML，历史和unknown锁住编辑保存", async () => {
  const submit = vi.fn();
  const props = {
    source,
    base: { expected_revision: 4 },
    locked: false,
    readOnly: false,
    onSubmit: submit,
    onDirty: vi.fn(),
  };
  const { rerender } = render(<SourceEditForm {...props} />);
  const input = screen.getByRole("textbox", { name: "来源标题" });
  fireEvent.change(input, { target: { value: "😀".repeat(513) } });
  fireEvent.click(screen.getByRole("checkbox", { name: "已获授权使用此正文" }));
  fireEvent.click(screen.getByRole("button", { name: "保存来源" }));
  expect(await screen.findByText("标题最多 512 个字符。")).toBeTruthy();
  expect((input as HTMLInputElement).value).toBe("😀".repeat(513));
  expect(submit).not.toHaveBeenCalled();
  rerender(<SourceEditForm {...props} locked />);
  expect(input.hasAttribute("disabled")).toBe(true);
  expect(
    screen.getByRole("button", { name: "保存来源" }).hasAttribute("disabled"),
  ).toBe(true);
});
