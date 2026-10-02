import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BibleEntryDialog } from "./bible-entry-dialog";
import type { BibleWriter } from "./use-bible-writer";
afterEach(cleanup);
function writer(overrides: Partial<BibleWriter> = {}): BibleWriter {
  return {
    ready: true,
    busy: false,
    intent: null,
    rejected: null,
    latest: null,
    error: undefined,
    storageError: undefined,
    locked: false,
    submit: vi.fn(),
    replay: vi.fn(),
    restoreStorage: vi.fn(),
    readLatest: vi.fn(),
    discardRejected: vi.fn(),
    ...overrides,
  };
}
const frame = {
  action: "create" as const,
  kind: "character" as const,
  revision: 0,
};
it("未知切换禁用字段/关闭，焦点转到原键核验，不触发新命令", async () => {
  const controller = writer(),
    close = vi.fn(),
    mounted = render(
      <BibleEntryDialog frame={frame} writer={controller} onClose={close} />,
    );
  const name = screen.getByRole("textbox", { name: "名称" });
  name.focus();
  const intent = {
    version: 1 as const,
    key: "11111111-1111-4111-8111-111111111111",
    origin: window.location.origin,
    actorId: "11111111-1111-4111-8111-111111111111",
    orgId: "22222222-2222-4222-8222-222222222222",
    projectId: "33333333-3333-4333-8333-333333333333",
    command: {
      action: "create" as const,
      kind: "character" as const,
      body: {
        expected_revision: 0,
        character: { name: "原角色", definition: {} },
      },
    },
  };
  mounted.rerender(
    <BibleEntryDialog
      frame={frame}
      writer={writer({ locked: true, intent, replay: controller.replay })}
      onClose={close}
    />,
  );
  const replay = screen.getByRole("button", {
    name: "人工使用原键和原输入核验",
  });
  await waitFor(() => expect(document.activeElement).toBe(replay));
  fireEvent.click(screen.getByRole("button", { name: "关闭设定编辑" }));
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(replay);
  expect(controller.replay).toHaveBeenCalledOnce();
  expect(controller.submit).not.toHaveBeenCalled();
});
it("dirty关闭提供保留草稿而无直接丢失，表单创建冻结CAS0", async () => {
  const controller = writer(),
    close = vi.fn();
  render(
    <BibleEntryDialog frame={frame} writer={controller} onClose={close} />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "名称" }), {
    target: { value: "完整角色😀" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存完整设定" }));
  await waitFor(() =>
    expect(controller.submit).toHaveBeenCalledWith({
      action: "create",
      kind: "character",
      body: {
        expected_revision: 0,
        character: { name: "完整角色😀", definition: {} },
      },
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "关闭设定编辑" }));
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "继续编辑草稿" }));
  expect(screen.getByRole("textbox", { name: "名称" })).toHaveProperty(
    "value",
    "完整角色😀",
  );
});
