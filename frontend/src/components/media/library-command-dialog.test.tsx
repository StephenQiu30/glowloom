import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LibraryCommandDialog } from "./library-command-dialog";
import type { LibraryWriter } from "./use-library-writer";
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
const scope = { kind: "personal" as const };
const actor = "11111111-1111-4111-8111-111111111111";
function writer(overrides: Partial<LibraryWriter> = {}): LibraryWriter {
  return {
    ready: true,
    busy: false,
    intent: null,
    rejected: null,
    latest: null,
    latestItems: [],
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
it("个人目录按40Unicode字符提交，表单只冻结原library CAS，Escape保留脏稿", async () => {
  const controller = writer(),
    close = vi.fn();
  render(
    <LibraryCommandDialog
      scope={scope}
      folders={[]}
      frame={{ action: "create_folder", revision: 4 }}
      writer={controller}
      onClose={close}
    />,
  );
  fireEvent.change(screen.getByLabelText("目录名称"), {
    target: { value: "😀".repeat(40) },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存目录" }));
  await waitFor(() =>
    expect(controller.submit).toHaveBeenCalledWith({
      scope,
      expected_revision: 4,
      action: "create_folder",
      folder: { parent_id: null, name: "😀".repeat(40), style: "", theme: "" },
    }),
  );
  fireEvent.keyDown(document, { key: "Escape" });
  expect(close).not.toHaveBeenCalled();
});
it("unknown切换时禁用表单和关闭，焦点落到原键核验，核验不生成新body", async () => {
  const body = {
    scope,
    action: "create_text" as const,
    expected_revision: 4,
    metadata: {
      plain_text: "原正文",
      title: "标题",
      folder_id: null,
      category: "material" as const,
      tags: [],
      source_label: "",
      note: "",
      favorite: false,
    },
  };
  const original = {
    version: 1 as const,
    key: actor,
    origin: window.location.origin,
    actorId: actor,
    orgId: actor,
    libraryId: actor,
    scope,
    body,
  };
  const controller = writer(),
    close = vi.fn();
  const mounted = render(
    <LibraryCommandDialog
      scope={scope}
      folders={[]}
      frame={{ action: "create_text", revision: 4 }}
      writer={controller}
      onClose={close}
    />,
  );
  const title = screen.getByLabelText("素材标题");
  title.focus();
  mounted.rerender(
    <LibraryCommandDialog
      scope={scope}
      folders={[]}
      frame={{ action: "create_text", revision: 4 }}
      writer={writer({
        locked: true,
        intent: original,
        replay: controller.replay,
      })}
      onClose={close}
    />,
  );
  const replay = screen.getByRole("button", {
    name: "人工使用原键核验素材库修改",
  });
  await waitFor(() => expect(document.activeElement).toBe(replay));
  expect(title.matches(":disabled")).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "关闭编辑窗口" }));
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(replay);
  expect(controller.replay).toHaveBeenCalledOnce();
  expect(controller.submit).not.toHaveBeenCalled();
});
it("批量回收确认列出全部冻结ID；项目移除明确保留原媒体", () => {
  const controller = writer();
  render(
    <LibraryCommandDialog
      scope={{ kind: "project", project_id: actor }}
      folders={[]}
      frame={{
        action: "remove_items",
        revision: 7,
        items: [{ id: actor, revision: 0 }],
      }}
      writer={controller}
      onClose={vi.fn()}
    />,
  );
  expect(screen.getByText(/原媒体和已有创作引用会保留/)).toBeTruthy();
  expect(screen.getByText(actor)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "确认移出项目素材库" }));
  expect(controller.submit).toHaveBeenCalledWith({
    scope: { kind: "project", project_id: actor },
    expected_revision: 7,
    action: "remove_items",
    items: [{ id: actor, revision: 0 }],
  });
});
