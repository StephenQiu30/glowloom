import { useState } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BatchDialog } from "@/components/governance/batch-dialog";
import {
  useBatchOperation,
  type BatchAction,
} from "@/components/governance/use-batch-operation";
vi.mock("@/api/identity", () => ({ reauthenticate: vi.fn() }));
afterEach(cleanup);
const action: BatchAction<{ id: string }> = {
  title: "测试操作",
  effect: "测试批次",
  label: (row) => row.id,
  prepare: async (row) => ({ row }),
  execute: async () => ({ message: "已完成" }),
};
function Example() {
  const [done, setDone] = useState(false);
  const { registerResultTrigger, ...batch } = useBatchOperation<{ id: string }>(
    () => setDone(true),
  );
  return (
    <>
      {!done ? (
        <button onClick={() => void batch.prepare([{ id: "one" }], action)}>
          发起批量操作
        </button>
      ) : null}
      <button ref={registerResultTrigger}>查看结果</button>
      <BatchDialog batch={batch} />
    </>
  );
}
it("成功项清空选择移除原按钮后，Escape 关闭结果仍恢复到可见入口", async () => {
  render(<Example />);
  const trigger = screen.getByRole("button", { name: "发起批量操作" });
  trigger.focus();
  fireEvent.click(trigger);
  const submit = await screen.findByRole("button", {
    name: "确认测试操作（1）",
  });
  fireEvent.click(submit);
  await screen.findByText("已完成");
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "查看结果" }),
    ),
  );
});
it("批量删除只有简短确认，成功后关闭且不展示逐项清单", async () => {
  function DeleteExample() {
    const batch = useBatchOperation<{ id: string }>(() => {});
    return (
      <>
        <button
          onClick={() =>
            void batch.prepare([{ id: "one" }, { id: "two" }], {
              ...action,
              title: "删除账号",
              effect: "删除后无法登录，作品保留。此操作不可恢复。",
              compact: true,
              destructive: true,
              prepare: async (row) => ({
                row,
                skip: row.id === "two" ? "当前账号不能删除" : undefined,
              }),
            })
          }
        >
          批量删除
        </button>
        <BatchDialog batch={batch} />
      </>
    );
  }
  render(<DeleteExample />);
  fireEvent.click(screen.getByRole("button", { name: "批量删除" }));
  await screen.findByRole("alertdialog", { name: "删除 1 个账号？" });
  expect(screen.getByText("已跳过 1 项")).toBeTruthy();
  expect(screen.queryByText("当前账号不能删除")).toBeNull();
  expect(screen.queryByRole("button", { name: "查看明细" })).toBeNull();
  expect(screen.queryByRole("table")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  expect(screen.queryByText("操作结果")).toBeNull();
});

it("精简操作的原入口消失后，自动关闭会恢复到筛选区", async () => {
  function CompactExample() {
    const [done, setDone] = useState(false);
    const { registerFallbackTrigger, ...batch } = useBatchOperation<{
      id: string;
    }>(() => setDone(true));
    return (
      <>
        <form
          aria-label="用户筛选"
          tabIndex={-1}
          ref={registerFallbackTrigger}
        />
        {!done ? (
          <button
            onClick={() =>
              void batch.prepare([{ id: "one" }], {
                ...action,
                title: "删除账号",
                compact: true,
              })
            }
          >
            批量删除
          </button>
        ) : null}
        <BatchDialog batch={batch} />
      </>
    );
  }
  render(<CompactExample />);
  const trigger = screen.getByRole("button", { name: "批量删除" });
  trigger.focus();
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("button", { name: "删除" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("form", { name: "用户筛选" }),
    ),
  );
});
