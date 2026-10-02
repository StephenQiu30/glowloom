import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SourceReorderDialog } from "./source-reorder-dialog";
afterEach(cleanup);
const lineages = [
  "11111111-1111-4111-8111-111111111111",
  "22222222-2222-4222-8222-222222222222",
];
const items = lineages.map((lineage, position) => ({
  id: lineage,
  source_lineage_id: lineage,
  source_revision: 1,
  source_kind: "chapter" as const,
  title: `章${position}`,
  status: "draft" as const,
  origin: "manual" as const,
  position,
  char_count: 1,
  content_hash: "a".repeat(64),
  rich_sha256: "b".repeat(64),
}));
it("位置草稿须确认完整集合与冻结CAS，关闭未保存需明确放弃", async () => {
  const submit = vi.fn();
  const close = vi.fn();
  render(
    <SourceReorderDialog
      items={items}
      base={{ expected_revision: 9 }}
      locked={false}
      onClose={close}
      onSubmit={submit}
      onDirty={vi.fn()}
    />,
  );
  fireEvent.change(
    screen.getByRole("textbox", { name: "选中来源移至第几个位置" }),
    { target: { value: "2" } },
  );
  fireEvent.click(screen.getByRole("button", { name: "应用位置草稿" }));
  fireEvent.click(screen.getByRole("button", { name: "关闭顺序调整" }));
  expect(close).not.toHaveBeenCalled();
  expect(
    screen.getByRole("button", { name: "放弃顺序草稿并关闭" }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "保存完整来源顺序" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({
      expected_revision: 9,
      source_lineage_ids: [lineages[1], lineages[0]],
    }),
  );
});
it("最新集合只在人工确认后替换，并以新CAS提交保留的顺序草稿", async () => {
  const third = {
    ...items[0],
    id: "33333333-3333-4333-8333-333333333333",
    source_lineage_id: "33333333-3333-4333-8333-333333333333",
    title: "新增章",
  };
  const fourth = {
    ...third,
    id: "44444444-4444-4444-8444-444444444444",
    source_lineage_id: "44444444-4444-4444-8444-444444444444",
  };
  const initial = [...items, { ...third, title: "章2" }];
  const submit = vi.fn();
  const accept = vi.fn();
  const props = {
    items: initial,
    base: { expected_revision: 9 },
    locked: false,
    onClose: vi.fn(),
    onSubmit: submit,
    onDirty: vi.fn(),
  };
  const { rerender } = render(<SourceReorderDialog {...props} />);
  fireEvent.change(
    screen.getByRole("textbox", { name: "选中来源移至第几个位置" }),
    { target: { value: "3" } },
  );
  fireEvent.click(screen.getByRole("button", { name: "应用位置草稿" }));
  const latestItems = [items[0], initial[2], fourth];
  rerender(
    <SourceReorderDialog
      {...props}
      locked
      latestItems={latestItems}
      onAcceptLatest={accept}
    />,
  );
  expect(screen.getByText(/已移除来源：章1/)).toBeTruthy();
  expect(screen.getByText(/新增来源：新增章/)).toBeTruthy();
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("button", { name: "采用最新集合并保留可用顺序草稿" }),
  );
  expect(accept).toHaveBeenCalledOnce();
  rerender(
    <SourceReorderDialog
      {...props}
      items={latestItems}
      base={{ expected_revision: 12 }}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "保存完整来源顺序" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({
      expected_revision: 12,
      source_lineage_ids: [
        third.source_lineage_id,
        lineages[0],
        fourth.source_lineage_id,
      ],
    }),
  );
});
