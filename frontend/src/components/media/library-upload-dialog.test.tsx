import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LibraryUploadDialog } from "./library-upload-dialog";
import { useLibraryUpload } from "./use-library-upload";
vi.mock("./use-library-upload", () => ({ useLibraryUpload: vi.fn() }));
const identity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  libraryId: "33333333-3333-5333-8333-333333333333",
  scope: { kind: "personal" as const },
};
const intent = {
  key: identity.actorId,
  fileName: "original.txt",
  byteSize: 4,
  sha256: "a".repeat(64),
  local_review_confirmed: true as const,
};
function uploader() {
  return {
    entries: [],
    ready: true,
    busy: false,
    locked: false,
    error: undefined,
    storageError: undefined,
    start: vi.fn(),
    replay: vi.fn(),
    restoreStorage: vi.fn(),
    discardRejected: vi.fn(),
    stopSending: vi.fn(),
  } satisfies ReturnType<typeof useLibraryUpload>;
}
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
it("提交控件移除后unknown焦点落到可选择原件入口，Escape/关闭锁；无自动重放", async () => {
  const state = uploader(),
    close = vi.fn();
  vi.mocked(useLibraryUpload).mockReturnValue(state);
  const props = {
    identity,
    onBusyChange: vi.fn(),
    onClose: close,
    onUploaded: vi.fn(),
  };
  const mounted = render(<LibraryUploadDialog {...props} />);
  fireEvent.change(screen.getByLabelText("本地素材原件"), {
    target: {
      files: [new File(["real"], intent.fileName, { type: "text/plain" })],
    },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  const submit = screen.getByRole("button", { name: "确认上传到个人素材库" });
  submit.focus();
  fireEvent.click(submit);
  vi.mocked(useLibraryUpload).mockReturnValue({
    ...state,
    locked: true,
    entries: [{ intent, status: "unknown", loaded: 0 }],
  });
  mounted.rerender(<LibraryUploadDialog {...props} />);
  const original = screen.getByLabelText(
    `重新选择确切原文件 ${intent.fileName}`,
  );
  await waitFor(() => expect(document.activeElement).toBe(original));
  fireEvent.keyDown(document, { key: "Escape" });
  fireEvent.click(screen.getByRole("button", { name: "关闭上传" }));
  expect(close).not.toHaveBeenCalled();
  expect(state.replay).not.toHaveBeenCalled();
});
it("人工review是实际上传前提，存储错误仅恢复按钮，无不可关闭的伪X入口", () => {
  const state = uploader();
  vi.mocked(useLibraryUpload).mockReturnValue({
    ...state,
    locked: true,
    storageError: "quota",
  });
  render(
    <LibraryUploadDialog
      identity={identity}
      onBusyChange={vi.fn()}
      onClose={vi.fn()}
      onUploaded={vi.fn()}
    />,
  );
  expect(
    screen.queryByRole("button", { name: "确认上传到个人素材库" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "恢复上传意图存储" }));
  expect(state.restoreStorage).toHaveBeenCalledOnce();
  expect(state.start).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
});
