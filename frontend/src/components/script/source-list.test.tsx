import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SourceList } from "./source-list";
afterEach(cleanup);
const source = {
  id: "11111111-1111-4111-8111-111111111111",
  source_lineage_id: "22222222-2222-4222-8222-222222222222",
  source_revision: 1,
  source_kind: "chapter" as const,
  title: "首章",
  status: "draft" as const,
  origin: "manual" as const,
  position: 0,
  char_count: 1,
  content_hash: "a".repeat(64),
  rich_sha256: "b".repeat(64),
};
it("来源摘要读取不假称全量，搜索/选中/分页错误可独立恢复", () => {
  const select = vi.fn();
  const next = vi.fn();
  render(
    <SourceList
      items={[source]}
      selected={undefined}
      locked={false}
      nextPage
      loadingNext={false}
      onSelect={select}
      onNext={next}
      pageError="分页失败"
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "打开来源 首章" }));
  expect(select).toHaveBeenCalledWith(source.source_lineage_id);
  expect(screen.getByText(/^已读取 1 个来源摘要/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重试读取下一页" }));
  expect(next).toHaveBeenCalledTimes(1);
  fireEvent.change(screen.getByRole("textbox", { name: "搜索已读取来源" }), {
    target: { value: "不存在" },
  });
  expect(screen.getByText("已读取的来源中没有匹配项。")).toBeTruthy();
});
