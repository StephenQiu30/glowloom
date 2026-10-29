import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useHomeDraftRecovery } from "@/app/_components/home-draft-recovery";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import type { LocalBranch } from "@/components/generation/draft-save";

const mocks = vi.hoisted(() => ({
  list: vi.fn<(key: string) => Promise<LocalBranch[]>>(),
  write: vi.fn<(branch: LocalBranch) => Promise<void>>(),
  remove: vi.fn<(branch: LocalBranch) => Promise<void>>(),
  scope: vi.fn<() => string | null>(),
}));

vi.mock("@/components/generation/draft-save", () => ({
  branchJournal: { list: mocks.list, write: mocks.write, remove: mocks.remove },
}));
vi.mock("@/lib/request", () => ({ getCreationScope: mocks.scope }));

const imageKey = "actor/space-a/home-input:image";
const agentKey = "actor/space-a/home-input:agent";
const legacyKey = "actor/space-a/home-input";

function textDraft(text: string): PromptDraft {
  const draft = emptyPromptDraft();
  draft.document.content = [
    { type: "paragraph", content: [{ type: "text", text }] },
  ];
  return draft;
}

function branch(
  contextKey: string,
  draft = textDraft("上次文字"),
): LocalBranch {
  return {
    inputRecovery: true,
    inputScope: "space",
    contextKey,
    replicaId: "previous-tab",
    localRecordRevision: 3,
    serverRevision: 0,
    draftId: contextKey.split("/")[2],
    ackedSeq: -1,
    snapshot: { contextKey, replicaId: "previous-tab", localSeq: 3, draft },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function Harness({
  contextKey,
  onChange,
  draft = textDraft("当前新输入"),
}: {
  contextKey: string | null;
  onChange: (draft: PromptDraft) => void;
  draft?: PromptDraft;
}) {
  const recovery = useHomeDraftRecovery(contextKey, onChange);
  return (
    <>
      {recovery.notice}
      <button onClick={() => recovery.change(draft)}>编辑文字</button>
    </>
  );
}

beforeEach(() => {
  vi.resetAllMocks();
  mocks.scope.mockReturnValue("space-a");
  mocks.list.mockResolvedValue([]);
  mocks.write.mockResolvedValue();
  mocks.remove.mockResolvedValue();
});
afterEach(cleanup);

describe("首页文字恢复的类型与作用域", () => {
  it.each(["agent", "video", "music", "audio", "avatar", "motion"])(
    "%s 只读取自己的类型 key，不读取旧共享草稿",
    async (type) => {
      const key = `actor/space-a/home-input:${type}`;
      render(<Harness contextKey={key} onChange={vi.fn()} />);
      await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));
      expect(mocks.list).toHaveBeenCalledWith(key);
      expect(mocks.list).not.toHaveBeenCalledWith(legacyKey);
    },
  );

  it("只有图片读取旧 home-input，并优先恢复图片自己的分支", async () => {
    const typed = branch(imageKey, textDraft("图片分支"));
    const legacy = branch(legacyKey, textDraft("旧共享分支"));
    mocks.list.mockImplementation(async (key) =>
      key === imageKey ? [typed] : [legacy],
    );
    const onChange = vi.fn();
    render(<Harness contextKey={imageKey} onChange={onChange} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "恢复上次文字" }),
    );
    await waitFor(() =>
      expect(onChange).toHaveBeenCalledWith(textDraft("图片分支")),
    );
    expect(mocks.list.mock.calls.slice(0, 2)).toEqual([
      [imageKey],
      [legacyKey],
    ]);
    await waitFor(() => expect(mocks.remove).toHaveBeenCalledWith(typed));
    expect(mocks.remove).not.toHaveBeenCalledWith(legacy);
  });

  it("页面内候选恢复旧文字和换行，但不复制引用、模型、参数或标签分配", async () => {
    const source = textDraft("保留正文");
    source.document.content[0].content!.push(
      { type: "reference", attrs: { referenceId: "ref-1" } },
      { type: "hardBreak" },
      { type: "text", text: "下一行" },
    );
    source.references = [
      {
        id: "ref-1",
        role: "reference_image",
        mediaKind: "image",
        order: 0,
        bindingGeneration: 1,
        source: {
          kind: "media",
          sourceId: "asset-1",
          sourceVersion: "version-1",
        },
      },
    ];
    source.modelVersion = "old-model";
    source.mode = "image2image";
    source.params = { output_count: 4, size: "4K" };
    source.labelState.highWater.image = 1;
    source.labelState.allocations["ref-1"] = { labelKind: "image", ordinal: 1 };
    const original = structuredClone(source);
    const saved = branch(legacyKey, source);
    mocks.list.mockImplementation(async (key) =>
      key === legacyKey ? [saved] : [],
    );
    const onChange = vi.fn();
    const first = render(<Harness contextKey={imageKey} onChange={onChange} />);
    await screen.findByRole("button", { name: "恢复上次文字" });
    first.unmount();
    render(<Harness contextKey={imageKey} onChange={onChange} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "恢复上次文字" }),
    );
    const expected = textDraft("保留正文");
    expected.document.content[0].content!.push(
      { type: "hardBreak" },
      { type: "text", text: "下一行" },
    );
    await waitFor(() => expect(onChange).toHaveBeenCalledWith(expected));
    expect(mocks.write).toHaveBeenCalledWith(
      expect.objectContaining({
        contextKey: imageKey,
        snapshot: expect.objectContaining({
          contextKey: imageKey,
          draft: expected,
        }),
      }),
    );
    await waitFor(() => expect(mocks.remove).toHaveBeenCalledWith(saved));
    expect(source).toEqual(original);
  });

  it("迟到的初始读取不在用户已编辑后重新出现恢复入口", async () => {
    const read = deferred<LocalBranch[]>();
    mocks.list.mockReturnValue(read.promise);
    const onChange = vi.fn();
    render(<Harness contextKey={agentKey} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "编辑文字" }));
    await act(async () => read.resolve([branch(agentKey)]));
    expect(screen.queryByRole("button", { name: "恢复上次文字" })).toBeNull();
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith(textDraft("当前新输入"));
  });

  it("旧作用域迟到读取不会出现在新作用域，也不会在返回旧类型后复活", async () => {
    const read = deferred<LocalBranch[]>();
    mocks.list.mockReturnValueOnce(read.promise);
    const onChange = vi.fn();
    const view = render(<Harness contextKey={agentKey} onChange={onChange} />);
    view.rerender(
      <Harness
        contextKey="actor/space-b/home-input:agent"
        onChange={onChange}
      />,
    );
    await act(async () => read.resolve([branch(agentKey)]));
    view.rerender(<Harness contextKey={agentKey} onChange={onChange} />);
    await act(async () => {});
    expect(screen.queryByRole("button", { name: "恢复上次文字" })).toBeNull();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("恢复重新读取迟到时，不覆盖用户已经输入的新文字", async () => {
    const saved = branch(agentKey);
    const read = deferred<LocalBranch[]>();
    mocks.list.mockResolvedValueOnce([saved]).mockReturnValueOnce(read.promise);
    const onChange = vi.fn();
    render(<Harness contextKey={agentKey} onChange={onChange} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "恢复上次文字" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "编辑文字" }));
    await act(async () => read.resolve([saved]));
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith(textDraft("当前新输入"));
    expect(mocks.remove).not.toHaveBeenCalled();
    expect(mocks.write).toHaveBeenCalledTimes(1);
  });

  it("恢复重新读取迟到时，不覆盖新类型的草稿或写回旧类型", async () => {
    const saved = branch(agentKey);
    const read = deferred<LocalBranch[]>();
    mocks.list.mockResolvedValueOnce([saved]).mockReturnValueOnce(read.promise);
    const onChange = vi.fn();
    const view = render(<Harness contextKey={agentKey} onChange={onChange} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "恢复上次文字" }),
    );
    view.rerender(<Harness contextKey={imageKey} onChange={onChange} />);
    await act(async () => read.resolve([saved]));
    expect(onChange).not.toHaveBeenCalled();
    expect(mocks.write).not.toHaveBeenCalled();
    expect(mocks.remove).not.toHaveBeenCalled();
  });

  it("恢复后写入失败保留唯一旧分支，提示不可用且文字仍可编辑", async () => {
    const saved = branch(agentKey);
    mocks.list.mockResolvedValue([saved]);
    mocks.write.mockRejectedValue(new Error("storage full"));
    const onChange = vi.fn();
    render(<Harness contextKey={agentKey} onChange={onChange} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "恢复上次文字" }),
    );
    await screen.findByText("文字恢复暂不可用，当前输入仍可编辑。");
    expect(onChange).toHaveBeenCalledWith(textDraft("上次文字"));
    expect(mocks.remove).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "编辑文字" }));
    expect(onChange).toHaveBeenLastCalledWith(textDraft("当前新输入"));
  });

  it("恢复分支重新读取后已消失时，不凭旧快照恢复或删除", async () => {
    mocks.list
      .mockResolvedValueOnce([branch(agentKey)])
      .mockResolvedValueOnce([]);
    const onChange = vi.fn();
    render(<Harness contextKey={agentKey} onChange={onChange} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "恢复上次文字" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "恢复上次文字" })).toBeNull(),
    );
    expect(onChange).not.toHaveBeenCalled();
    expect(mocks.write).not.toHaveBeenCalled();
    expect(mocks.remove).not.toHaveBeenCalled();
  });

  it.each([null, "actor/space-b/home-input:agent"])(
    "作用域 %s 与当前空间失配时不写入 journal",
    async (contextKey) => {
      const onChange = vi.fn();
      render(<Harness contextKey={contextKey} onChange={onChange} />);
      fireEvent.click(screen.getByRole("button", { name: "编辑文字" }));
      await act(async () => {});
      expect(onChange).toHaveBeenCalledWith(textDraft("当前新输入"));
      expect(mocks.write).not.toHaveBeenCalled();
      expect(mocks.remove).not.toHaveBeenCalled();
    },
  );
});
