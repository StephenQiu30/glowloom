import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AssetPickerModal } from "@/components/canvas/runtime/components/assets/picker/asset-picker-modal";
const harness = vi.hoisted(() => ({
  scope: 1,
  session: 1,
  payload: vi.fn(),
  props: {} as {
    onAdd: (assets: API.AssetSummary[]) => Promise<void>;
    onUnifiedSubject?: (
      subject: import("@/components/subjects/unified-contracts").UnifiedSubject,
    ) => Promise<void>;
    selectionLimit?: number;
    confirmLabel?: string;
  },
}));
vi.mock("@/lib/request", () => ({
  getScopeGeneration: () => harness.scope,
  getSessionGeneration: () => harness.session,
}));
vi.mock("@/components/media/project-asset-dialog", () => ({
  ProjectAssetDialog: (props: typeof harness.props) => {
    harness.props = props;
    return <div>项目素材选择</div>;
  },
}));
vi.mock(
  "@/components/canvas/runtime/components/assets/picker/library-picker-content",
  () => ({
    canvasAssetPayload: harness.payload,
    LibraryPickerContent: () => null,
  }),
);
beforeEach(() => {
  harness.payload.mockReset();
  harness.scope = 1;
  harness.session = 1;
});
afterEach(cleanup);
const assets = [{ id: "one" }, { id: "two" }] as API.AssetSummary[];
it("所有预览都解析成功后才插入；任一失败不留下部分节点", async () => {
  const insert = vi.fn();
  const close = vi.fn();
  render(
    <AssetPickerModal
      open
      projectId="project"
      onInsert={insert}
      onClose={close}
    />,
  );
  harness.payload
    .mockResolvedValueOnce({ kind: "image", title: "one" })
    .mockRejectedValueOnce(new Error("预览失效"));
  await expect(harness.props.onAdd(assets)).rejects.toThrow("预览失效");
  expect(insert).not.toHaveBeenCalled();
  expect(close).not.toHaveBeenCalled();
  harness.payload
    .mockResolvedValueOnce({ kind: "image", title: "one" })
    .mockResolvedValueOnce({ kind: "video", title: "two" });
  await act(() => harness.props.onAdd(assets));
  expect(insert.mock.calls.map((call) => call[0].title)).toEqual([
    "one",
    "two",
  ]);
  expect(close).toHaveBeenCalledOnce();
});
it("关闭选择器后取消未完成请求，不插入迟到结果", async () => {
  const insert = vi.fn();
  let resolve!: (value: unknown) => void;
  harness.payload.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const props = { projectId: "project", onInsert: insert, onClose: vi.fn() };
  const view = render(<AssetPickerModal open {...props} />);
  const pending = harness.props.onAdd([assets[0]]);
  const signal = harness.payload.mock.calls[0][2] as AbortSignal;
  view.rerender(<AssetPickerModal open={false} {...props} />);
  expect(signal.aborted).toBe(true);
  resolve({ kind: "image", title: "one" });
  await act(() => pending);
  expect(insert).not.toHaveBeenCalled();
});
it("账号范围改变后拒绝迟到素材，替换入口明确限制单选", async () => {
  const insert = vi.fn();
  let resolve!: (value: unknown) => void;
  harness.payload.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  render(
    <AssetPickerModal
      open
      projectId="project"
      kinds={["image"]}
      onInsert={insert}
      onClose={vi.fn()}
    />,
  );
  expect(harness.props.selectionLimit).toBe(1);
  expect(harness.props.confirmLabel).toBe("替换素材");
  const pending = harness.props.onAdd([assets[0]]);
  harness.session++;
  resolve({ kind: "image", title: "one" });
  await act(() => pending);
  expect(insert).not.toHaveBeenCalled();
});

it("主体固定版本导入在历史恢复新epoch、空间切换、关闭后均拒绝", async () => {
  const insert = vi.fn();
  let epoch = "old";
  const props = {
    projectId: "project",
    getDocumentEpoch: () => epoch,
    onInsert: insert,
    onClose: vi.fn(),
  };
  const view = render(<AssetPickerModal open {...props} />);
  const subject = {
    id: "subject",
    version_id: "version",
    name: "主演",
  } as import("@/components/subjects/unified-contracts").UnifiedSubject;
  const confirm = harness.props.onUnifiedSubject!;
  epoch = "restored";
  await expect(confirm(subject)).rejects.toThrow("画布或创作空间已变化");
  expect(insert).not.toHaveBeenCalled();
  view.rerender(<AssetPickerModal open={false} {...props} />);
  view.rerender(<AssetPickerModal open {...props} />);
  await act(() => harness.props.onUnifiedSubject!(subject));
  expect(insert).toHaveBeenCalledExactlyOnceWith({ kind: "subject", subject });
  harness.scope++;
  await expect(harness.props.onUnifiedSubject!(subject)).rejects.toThrow();
  view.unmount();
  await expect(harness.props.onUnifiedSubject!(subject)).rejects.toThrow();
  expect(insert).toHaveBeenCalledOnce();
});
it("媒体预览等待期间epoch变化不会把素材写入恢复后的画布", async () => {
  let epoch = "one";
  let finish!: (value: unknown) => void;
  const insert = vi.fn();
  harness.payload.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  render(
    <AssetPickerModal
      open
      projectId="project"
      getDocumentEpoch={() => epoch}
      onInsert={insert}
      onClose={vi.fn()}
    />,
  );
  const pending = harness.props.onAdd([assets[0]]);
  epoch = "two";
  finish({ kind: "image", title: "one" });
  await act(() => pending);
  expect(insert).not.toHaveBeenCalled();
});

it("切换项目后旧确认闭包不能写入新项目", async () => {
  const insert = vi.fn();
  const view = render(
    <AssetPickerModal
      open
      projectId="old"
      onInsert={insert}
      onClose={vi.fn()}
    />,
  );
  const oldConfirm = harness.props.onUnifiedSubject!;
  view.rerender(
    <AssetPickerModal
      open
      projectId="new"
      onInsert={insert}
      onClose={vi.fn()}
    />,
  );
  await expect(
    oldConfirm({
      id: "subject",
    } as import("@/components/subjects/unified-contracts").UnifiedSubject),
  ).rejects.toThrow();
  expect(insert).not.toHaveBeenCalled();
});
