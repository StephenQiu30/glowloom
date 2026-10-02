import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SourceImportDialog } from "./source-import-dialog";
afterEach(cleanup);
it("整批非法保留JSON且不提交；有效批次和版权一次提交原CAS", async () => {
  const submit = vi.fn();
  render(
    <SourceImportDialog
      open
      locked={false}
      base={{ expected_revision: 7 }}
      onClose={vi.fn()}
      onSubmit={submit}
      onDirty={vi.fn()}
    />,
  );
  const area = screen.getByRole("textbox", { name: "完整章节 JSON" });
  fireEvent.change(area, { target: { value: '[{"type":"image"}]' } });
  fireEvent.click(
    screen.getByRole("checkbox", { name: "已获授权使用全部章节正文" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "原子导入全部章节" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect((area as HTMLTextAreaElement).value).toContain("image");
  expect(submit).not.toHaveBeenCalled();
  fireEvent.change(area, {
    target: {
      value: JSON.stringify([
        {
          source_kind: "chapter",
          title: "章",
          status: "draft",
          document: { type: "doc" },
          provenance: {},
        },
      ]),
    },
  });
  fireEvent.click(screen.getByRole("button", { name: "原子导入全部章节" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    expected_revision: 7,
    rights_confirmed: true,
    sources: [{ title: "章" }],
  });
});
it("未知锁住模式/JSON/版权/关闭和新提交", () => {
  render(
    <SourceImportDialog
      open
      locked
      base={{ expected_revision: 7 }}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
      onDirty={vi.fn()}
    />,
  );
  expect(
    screen
      .getByRole("textbox", { name: "完整章节 JSON" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "原子导入全部章节" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen.getByRole("button", { name: "关闭导入" }).hasAttribute("disabled"),
  ).toBe(true);
});
