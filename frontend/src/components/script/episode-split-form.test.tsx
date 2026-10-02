import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { EpisodeSplitForm } from "./episode-split-form";
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
const base = {
  version_id: "11111111-1111-4111-8111-111111111111",
  expected_revision: 7,
  expected_split_revision: 2,
  candidate_set_id: "22222222-2222-4222-8222-222222222222",
};
const boundaries = [{ seq_no: 1, title: "第一集", span_start: 0, span_end: 9 }];
it("规则未给边界时仍可由用户明确划首集，空正文不造零宽集", async () => {
  const submit = vi.fn();
  const ui = render(
    <EpisodeSplitForm
      base={base}
      initialBoundaries={[]}
      charCount={9}
      locked={false}
      onDirty={vi.fn()}
      onSubmit={submit}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "以完整正文建立第一集草稿" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "确认完整分集" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({
      ...base,
      boundaries: [{ seq_no: 1, title: "第 1 集", span_start: 0, span_end: 9 }],
      ack_invalidate: false,
    }),
  );
  ui.unmount();
  render(
    <EpisodeSplitForm
      base={base}
      initialBoundaries={[]}
      charCount={0}
      locked={false}
      onDirty={vi.fn()}
      onSubmit={submit}
    />,
  );
  expect(
    screen.queryByRole("button", { name: "以完整正文建立第一集草稿" }),
  ).toBeNull();
  expect(
    screen.getByRole("button", { name: "确认完整分集" }).matches(":disabled"),
  ).toBe(true);
});
it("人工拆分完整集合与原CAS一次提交，不默改原候选或遗漏Unicode区间", async () => {
  const submit = vi.fn();
  render(
    <EpisodeSplitForm
      base={base}
      initialBoundaries={boundaries}
      charCount={9}
      locked={false}
      onSubmit={submit}
      onDirty={vi.fn()}
    />,
  );
  fireEvent.change(
    screen.getByRole("spinbutton", { name: "插入新分集的正文位置" }),
    { target: { value: "4" } },
  );
  fireEvent.click(screen.getByRole("button", { name: "在此位置拆分来源区间" }));
  fireEvent.click(screen.getByRole("button", { name: "确认完整分集" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({
      ...base,
      boundaries: [
        { ...boundaries[0], span_end: 4 },
        { seq_no: 2, title: "第 2 集", span_start: 4, span_end: 9 },
      ],
      ack_invalidate: false,
    }),
  );
  expect(boundaries[0].span_end).toBe(9);
});
it("草稿错误保留原输入并拒绝遗漏终点，unknown锁住边界与确认", async () => {
  const submit = vi.fn();
  const props = {
    base,
    initialBoundaries: boundaries,
    charCount: 9,
    locked: false,
    onSubmit: submit,
    onDirty: vi.fn(),
  };
  const { rerender } = render(<EpisodeSplitForm {...props} />);
  const end = screen.getByRole("spinbutton", { name: "第1集正文终点" });
  fireEvent.change(end, { target: { value: "8" } });
  fireEvent.click(screen.getByRole("button", { name: "确认完整分集" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(submit).not.toHaveBeenCalled();
  expect((end as HTMLInputElement).value).toBe("8");
  rerender(<EpisodeSplitForm {...props} locked />);
  expect(end.hasAttribute("disabled")).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "确认完整分集" })
      .hasAttribute("disabled"),
  ).toBe(true);
});
