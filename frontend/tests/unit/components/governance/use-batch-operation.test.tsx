import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import {
  useBatchOperation,
  type BatchAction,
} from "@/components/governance/use-batch-operation";
import { RequestError } from "@/lib/request";
vi.mock("@/api/identity", () => ({
  reauthenticate: vi.fn().mockResolvedValue(undefined),
}));
type Row = { id: string; revision: number };
function action(): BatchAction<Row> {
  return {
    title: "停用",
    effect: "测试",
    label: (r) => r.id,
    prepare: vi.fn(async (row) => ({ row })),
    execute: vi.fn(async () => ({ message: "完成" })),
    recover: vi.fn(async () => undefined),
  };
}
describe("批量操作生命周期", () => {
  it("双击只发送一次，关闭弹窗不丢失执行状态", async () => {
    let finish!: (v: { message: string }) => void;
    const a = action();
    a.execute = vi.fn(
      () =>
        new Promise<{ message: string }>((resolve) => {
          finish = resolve;
        }),
    );
    const settled = vi.fn();
    const { result } = renderHook(() => useBatchOperation<Row>(settled));
    await act(() => result.current.prepare([{ id: "a", revision: 1 }], a));
    let run!: Promise<void>;
    act(() => {
      run = result.current.run("reason", "");
      void result.current.run("reason", "");
    });
    await waitFor(() => expect(a.execute).toHaveBeenCalledTimes(1));
    act(() => result.current.setOpen(false));
    await act(async () => {
      finish({ message: "完成" });
      await run;
    });
    expect(result.current.open).toBe(false);
    expect(result.current.entries[0].status).toBe("success");
    expect(settled).toHaveBeenCalledWith(["a"]);
  });
  it("未知结果恢复保持原 ID、版本和原因，成功项不会再次执行", async () => {
    const a = action();
    a.execute = vi
      .fn()
      .mockResolvedValueOnce({ message: "完成" })
      .mockRejectedValueOnce(new RequestError("network", 0))
      .mockResolvedValue({ message: "完成" });
    const { result } = renderHook(() => useBatchOperation<Row>(() => {}));
    await act(() =>
      result.current.prepare(
        [
          { id: "a", revision: 1 },
          { id: "b", revision: 3 },
        ],
        a,
      ),
    );
    const key = result.current.entries[1].requestId;
    await act(() => result.current.run("原始原因", ""));
    expect(result.current.unresolved).toBe(true);
    await act(() =>
      result.current.prepare([{ id: "c", revision: 5 }], action()),
    );
    expect(result.current.entries).toHaveLength(2);
    await act(() => result.current.run("不应替换原始原因", "", true));
    expect(a.execute).toHaveBeenCalledTimes(3);
    expect(vi.mocked(a.execute).mock.calls[2][0]).toMatchObject({
      id: "b",
      requestId: key,
      row: { revision: 3 },
    });
    expect(vi.mocked(a.execute).mock.calls[2][1]).toBe("原始原因");
  });
  it("失败项重新读取后需再次确认并生成新 ID", async () => {
    const a = action();
    a.execute = vi
      .fn()
      .mockRejectedValue(new RequestError("revision_conflict", 409));
    a.prepare = vi.fn(async (row, _signal, refresh) => ({
      row: refresh ? { ...row, revision: 2 } : row,
    }));
    const { result } = renderHook(() => useBatchOperation<Row>(() => {}));
    await act(() => result.current.prepare([{ id: "a", revision: 1 }], a));
    const old = result.current.entries[0].requestId;
    await act(() => result.current.run("原因", ""));
    await act(async () => {
      await result.current.retry();
    });
    expect(result.current.entries[0]).toMatchObject({
      status: "ready",
      row: { revision: 2 },
    });
    expect(result.current.entries[0].requestId).not.toBe(old);
    expect(a.execute).toHaveBeenCalledTimes(1);
    expect(result.current.submitted).toBe(false);
  });
  it("查到原回执后移除选择且不重放写入", async () => {
    const a = action();
    a.execute = vi.fn().mockRejectedValue(new RequestError("network", 0));
    a.recover = vi.fn(async () => ({ message: "原操作成功" }));
    const settled = vi.fn();
    const { result } = renderHook(() => useBatchOperation<Row>(settled));
    await act(() => result.current.prepare([{ id: "a", revision: 1 }], a));
    await act(() => result.current.run("原因", ""));
    await act(() => result.current.recover());
    expect(result.current.entries[0].status).toBe("success");
    expect(a.execute).toHaveBeenCalledTimes(1);
    expect(settled).toHaveBeenLastCalledWith(["a"]);
  });
});

it("权限失效隐藏批量上下文并停止后续写入", async () => {
  const a = action();
  a.execute = vi.fn().mockRejectedValue(new RequestError("forbidden", 403));
  const { result } = renderHook(() => useBatchOperation<Row>(() => {}));
  await act(() =>
    result.current.prepare(
      [
        { id: "a", revision: 1 },
        { id: "b", revision: 1 },
      ],
      a,
    ),
  );
  await act(() => result.current.run("原因", ""));
  expect(result.current.denied).toBe(true);
  expect(a.execute).toHaveBeenCalledTimes(1);
});

it("预检权限失效也关闭私有上下文，不允许发起写入", async () => {
  const a = action();
  a.prepare = vi.fn().mockRejectedValue(new RequestError("forbidden", 403));
  const { result } = renderHook(() => useBatchOperation<Row>(() => {}));
  await act(() => result.current.prepare([{ id: "a", revision: 1 }], a));
  expect(result.current.denied).toBe(true);
  expect(a.execute).not.toHaveBeenCalled();
});
