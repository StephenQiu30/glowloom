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
