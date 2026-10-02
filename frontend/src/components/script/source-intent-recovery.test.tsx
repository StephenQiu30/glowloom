import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SourceIntentRecovery } from "./source-intent-recovery";
afterEach(cleanup);
const intent = {
  version: 1 as const,
  key: "11111111-1111-4111-8111-111111111111",
  origin: "http://127.0.0.1:3000",
  actorId: "22222222-2222-4222-8222-222222222222",
  orgId: "33333333-3333-4333-8333-333333333333",
  projectId: "44444444-4444-4444-8444-444444444444",
  action: "create" as const,
  body: {
    expected_revision: 4,
    rights_confirmed: true as const,
    source_kind: "chapter" as const,
    title: "章节",
    status: "draft" as const,
    document: { type: "doc" as const },
    provenance: {},
  },
};
it("未知转移焦点到原键核验，不换CAS/key且busy禁止重放", () => {
  const replay = vi.fn();
  const { rerender } = render(
    <>
      <button>原保存</button>
      <SourceIntentRecovery
        intent={null}
        busy={false}
        onReplay={replay}
        onStorageCheck={vi.fn()}
      />
    </>,
  );
  screen.getByRole("button", { name: "原保存" }).focus();
  rerender(
    <>
      <button disabled>原保存</button>
      <SourceIntentRecovery
        intent={intent}
        busy={false}
        onReplay={replay}
        onStorageCheck={vi.fn()}
      />
    </>,
  );
  const button = screen.getByRole("button", { name: "人工使用原键核验保存" });
  expect(document.activeElement).toBe(button);
  fireEvent.click(button);
  expect(replay).toHaveBeenCalledTimes(1);
  expect(screen.getByText(/原脚本版本 4/)).toBeTruthy();
  rerender(
    <SourceIntentRecovery
      intent={intent}
      busy
      onReplay={replay}
      onStorageCheck={vi.fn()}
    />,
  );
  expect(
    screen
      .getByRole("button", { name: "正在核验原保存…" })
      .hasAttribute("disabled"),
  ).toBe(true);
});
it("跨刷新核验前可人工查看完整原请求，不拿当前来源替换旧正文", () => {
  render(
    <SourceIntentRecovery
      intent={intent}
      busy={false}
      onReplay={vi.fn()}
      onStorageCheck={vi.fn()}
    />,
  );
  const details = screen.getByText("查看完整原请求正文").closest("details")!;
  expect(screen.queryByLabelText("完整原请求正文")).toBeNull();
  details.open = true;
  fireEvent(details, new Event("toggle"));
  expect(
    JSON.parse(screen.getByLabelText("完整原请求正文").textContent!),
  ).toEqual(intent.body);
});
it("存储失败仅允许先恢复存储，禁用新写入提示无丢弃旧键按钮", () => {
  render(
    <SourceIntentRecovery
      intent={null}
      storageError="存储不可读"
      busy={false}
      onReplay={vi.fn()}
      onStorageCheck={vi.fn()}
    />,
  );
  expect(
    screen.getByRole("button", { name: "恢复存储并读取原意图" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: /放弃|新键/ })).toBeNull();
});
